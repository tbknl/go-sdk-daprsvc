package daprsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/tbknl/go-functils"
	"github.com/tbknl/go-johanson"
)

type PubsubOptions struct {
	RawPayload   bool // NOTE: If true, instruct dapr daemon to always wrap message in a cloud-event.
	NoCloudEvent bool // NOTE: if true, do not parse incoming message data before sending to handler.
	// TODO: Support for matcing rules and priorities.
}

type MessageFields struct {
	Source    string
	Type      string
	Schema    string
	Subject   string
	Timestamp time.Time
}

type Message struct {
	PubsubName  string
	Topic       string
	Id          string
	Data        []byte
	ContentType string
	Metadata    map[string]string
	Fields      MessageFields
	Trace       struct {
		Id     string
		Parent string
		State  string
	}
}

var regexDataContentTypeJson = regexp.MustCompile(`^[^/]+/([^/]+\+)?json$`)

func (msg Message) ContainsJsonData() bool {
	return msg.ContentType == "" || regexDataContentTypeJson.MatchString(msg.ContentType)
}

func (msg Message) Json(v any) error {
	if !msg.ContainsJsonData() {
		return fmt.Errorf("Message has non-json content-type '%s'.", msg.ContentType)
	}
	return json.Unmarshal(msg.Data, v)
}

type MessageResult interface {
	private() // Can't be implemented outside this package.
	Success() bool
	Retry() bool
	Drop() bool
	Error() error
}

const (
	messageSuccess = iota
	messageRetry
	messageDrop
)

type messageResultImpl struct {
	err    error
	result int
}

func (mri messageResultImpl) private() {}

func (mri messageResultImpl) Success() bool {
	return mri.result == messageSuccess
}

func (mri messageResultImpl) Retry() bool {
	return mri.result == messageRetry
}

func (mri messageResultImpl) Drop() bool {
	return mri.result == messageDrop
}

func (mri messageResultImpl) Error() error {
	return mri.err
}

func MessageResultSuccess() MessageResult {
	return messageResultImpl{}
}

func MessageResultRetry(err error) MessageResult {
	if err == nil {
		err = errors.New("")
	}
	return messageResultImpl{err, messageRetry}
}

func MessageResultDrop(err error) MessageResult {
	if err == nil {
		err = errors.New("")
	}
	return messageResultImpl{err, messageDrop}
}

type MessageHandler = func(ctx context.Context, message Message) MessageResult

type pubsubEntry struct {
	pubsubName     string
	topic          string
	options        PubsubOptions
	messageHandler MessageHandler
}

func (entry pubsubEntry) constructRoute() string {
	return fmt.Sprintf("/%s/%s", entry.pubsubName, entry.topic) // TODO: Add optional matching rule and priority to route somehow.
}

type pubsub struct {
	name    string
	entries []pubsubEntry
}

func (ps *pubsub) RegisterMessageHandler(topic string, options PubsubOptions, handler MessageHandler) {
	ps.entries = append(ps.entries, pubsubEntry{
		pubsubName:     ps.name,
		topic:          topic,
		options:        options,
		messageHandler: handler,
	})
}

type pubsubMap map[string]*pubsub

type events struct {
	pubsubs pubsubMap
}

func (ev *events) writePubsubConfigData(w io.Writer, routePrefix string) error {
	jsw := johanson.NewStreamWriter(w)
	jsw.Array(func(psa johanson.V) {
		for _, ps := range ev.pubsubs {
			for _, entry := range ps.entries {
				psa.Object(func(pso johanson.K) {
					pso.Item("pubsubname").String(entry.pubsubName)
					pso.Item("topic").String(entry.topic)
					pso.Item("route").String(routePrefix + entry.constructRoute())
					pso.Item("metadata").Object(func(mdo johanson.K) {
						if entry.options.RawPayload {
							mdo.Item("rawPayload").String("true")
						}
					})
				})
			}
		}
	})

	return jsw.Error()
}

func (ev *events) pubsubEntriesWithRoutes() (result []struct {
	route string
	entry pubsubEntry
}) {
	for _, ps := range ev.pubsubs {
		for _, entry := range ps.entries {
			result = append(result, struct {
				route string
				entry pubsubEntry
			}{
				route: entry.constructRoute(),
				entry: entry,
			})
		}
	}
	return
}

func (ev *events) NewPubsub(name string) *pubsub {
	ps := &pubsub{name: name}
	if ev.pubsubs == nil {
		ev.pubsubs = make(pubsubMap, 10)
	}
	ev.pubsubs[name] = ps
	return ps
}

var metadataFromHeader = functils.Pipe5(
	functils.PipeInputType[http.Header],
	functils.MapEntries,
	functils.SliceFilter(func(h functils.KV[string, []string]) bool {
		return strings.HasPrefix(strings.ToLower(h.Key), "metadata.")
	}),
	functils.SliceTransform(func(h functils.KV[string, []string]) functils.KV[string, string] {
		return functils.KV[string, string]{h.Key, h.Value[0]}
	}),
	functils.MapFromEntries,
)

type jsonValueBuf []byte

func (buf *jsonValueBuf) UnmarshalJSON(bytes []byte) error {
	*buf = bytes
	return nil
}

func makeEventMessageHandler(entry pubsubEntry) httprouter.Handle {
	messageParseFail := func(w http.ResponseWriter, err error) {
		errMsg := fmt.Errorf("Failed to parse event message for pubsub '%s' on topic '%s': %w", entry.pubsubName, entry.topic, err)
		log.Println(errMsg) // TODO: Allow to inject logger.
		w.Header().Add("Content-Type", "text/plain")
		w.WriteHeader(400)
		w.Write([]byte(errMsg.Error()))
	}

	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		body, bodyErr := io.ReadAll(r.Body)
		if bodyErr != nil {
			messageParseFail(w, fmt.Errorf("Failed to read message body: %w", bodyErr))
			return
		}

		metadata := metadataFromHeader(r.Header)

		msg := Message{
			PubsubName:  entry.pubsubName,
			Topic:       entry.topic,
			Id:          "",
			Data:        body,
			ContentType: "",
			Metadata:    metadata,
		}

		if !entry.options.NoCloudEvent {
			if contentType := r.Header.Get("Content-Type"); contentType != "application/cloudevents+json" {
				messageParseFail(w, fmt.Errorf("Message does not have a cloud-event content-type: %s", contentType))
				return
			}

			cloudEvent := struct {
				Id              string        `json="id"`
				Source          string        `json="source"`
				Specversion     string        `json="specversion"`
				Type            string        `json="type"`
				Datacontenttype *string       `json="datacontenttype"`
				Dataschema      *string       `json="dataschema"`
				Subject         *string       `json="subject"`
				Time            *string       `json="time"`
				Data            *jsonValueBuf `json="data"`
				Data_base64     *string       `json="data_base64"`

				// Extension fields from dapr daemon:
				Pubsubname  string `json="pubsubname"`
				Topic       string `json="topic"`
				Traceid     string `json="traceid"`
				Traceparent string `json="traceparent"`
				Tracestate  string `json="tracestate"`
			}{}

			jsonErr := json.Unmarshal(body, &cloudEvent)
			if jsonErr != nil {
				messageParseFail(w, fmt.Errorf("Failed to unmarshal cloud-event json: %w", jsonErr))
				return
			}

			if cloudEvent.Specversion != "1.0" {
				messageParseFail(w, fmt.Errorf("Unknown cloud-event spec version '%s'.", cloudEvent.Specversion))
				return
			}

			if cloudEvent.Pubsubname != entry.pubsubName || cloudEvent.Topic != entry.topic {
				messageParseFail(w, fmt.Errorf("Message arrived at wrong destination (%s/%s) instead of (%s/%s).", entry.pubsubName, entry.topic, cloudEvent.Pubsubname, cloudEvent.Topic))
				return
			}

			msg.Id = cloudEvent.Id

			msg.ContentType = functils.DefaultOnNil(cloudEvent.Datacontenttype)

			if msg.ContainsJsonData() {
				msg.Data = *cloudEvent.Data
			} else if dataBase64 := cloudEvent.Data_base64; dataBase64 != nil {
				data, err := base64.StdEncoding.DecodeString(*dataBase64)
				if err != nil {
					messageParseFail(w, fmt.Errorf("Failed to decode cloud-event base64 data."))
					return
				}
				msg.Data = data
			} else {
				messageParseFail(w, fmt.Errorf("Cloud-event data does not match content type."))
				return
			}

			msg.Fields = MessageFields{
				Source:    cloudEvent.Source,
				Type:      cloudEvent.Type,
				Schema:    functils.DefaultOnNil(cloudEvent.Dataschema),
				Subject:   functils.DefaultOnNil(cloudEvent.Subject),
				Timestamp: functils.DefaultOnErr(func(t string) (time.Time, error) { return time.Parse(time.RFC3339, t) })(functils.DefaultOnNil(cloudEvent.Time)),
			}

			msg.Trace.Id = cloudEvent.Traceid
			msg.Trace.Parent = cloudEvent.Traceparent
			msg.Trace.State = cloudEvent.Tracestate
		}

		result := entry.messageHandler(r.Context(), msg)

		switch {
		case result.Success():
			// TODO: Log info.
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"SUCCESS"}`))
		case result.Retry():
			w.Header().Add("Content-Type", "application/json")
			// TODO: Log error.
			retryErr := result.Error()
			w.WriteHeader(500)
			jw := johanson.NewStreamWriter(w)
			jw.Object(func(o johanson.K) {
				o.Item("status").String("RETRY")
				if retryErr != nil {
					o.Item("error").String(retryErr.Error())
				}
			})
		case result.Drop():
			w.Header().Add("Content-Type", "application/json")
			// TODO: Log error.
			dropErr := result.Error()
			w.WriteHeader(400)
			jw := johanson.NewStreamWriter(w)
			jw.Object(func(o johanson.K) {
				o.Item("status").String("DROP")
				if dropErr != nil {
					o.Item("error").String(dropErr.Error())
				}
			})
		default:
			w.Header().Add("Content-Type", "text/plain")
			w.WriteHeader(400)
			w.Write([]byte("Invalid message handler result."))
		}
	}
}
