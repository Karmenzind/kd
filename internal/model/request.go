package model

type QueryDaemon struct {
	Action string
	Q      Result
}

type DaemonResponse struct {
	R     *Result
	Error string
	Found bool
}
