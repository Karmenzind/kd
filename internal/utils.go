package internal

import "github.com/Karmenzind/kd/internal/model"

var IS_SERVER = false

func buildResult(q string, ilt bool) *model.Result {
	return &model.Result{
		BaseResult: &model.BaseResult{
			Query:      q,
			IsLongText: ilt,
		},
	}
}
