package daprsvc

type daprSvc struct {
	invocation
	events
}

func New() *daprSvc {
	return &daprSvc{}
}
