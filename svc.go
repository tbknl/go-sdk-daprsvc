package daprsvc

type daprSvc struct {
	invocation
}

func New() *daprSvc {
	return &daprSvc{}
}
