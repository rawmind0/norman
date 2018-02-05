package api

import "github.com/rawmind0/norman/types"

type ResponseWriter interface {
	Write(apiContext *types.APIContext, code int, obj interface{})
}
