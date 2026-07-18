package model

const DaemonProtocolVersion = 1

type TCPQuery struct {
	Action          string
	ProtocolVersion int
	B               *BaseResult
}

func (q *TCPQuery) GetResult() *Result {
	return &Result{BaseResult: q.B}
}

type DaemonResponse struct {
	R     *Result
	Error string
	Ping  *DaemonPing `json:",omitempty"`

	Base *BaseResult
}

type DaemonPing struct {
	Available       bool
	PID             int
	Version         string
	ProtocolVersion int
	StartTime       int64
}

func (dr *DaemonResponse) GetResult() *Result {
	// json传递中被抹去，重新赋值
	dr.R.BaseResult = dr.Base
	return dr.R
}
