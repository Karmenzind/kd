package model

type QueryDaemon struct {
	Action string
	Q      Result
}

type DaemonResponse struct {
	R *Result

	Error      string
	Found      bool

	// 需要通过Tcp传递，又不能入库的字段
	IsLongText bool
}
