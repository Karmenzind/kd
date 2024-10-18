package model

type TCPQuery struct {
    Action string
    B      *BaseResult
}

func (q *TCPQuery) GetResult() *Result {
    return &Result{BaseResult: q.B}
}

type DaemonResponse struct {
    R     *Result
    Error string

    Base *BaseResult
}

func (dr *DaemonResponse) GetResult() *Result {
    // json传递中被抹去，重新赋值
    dr.R.BaseResult = dr.Base
    return dr.R
}
