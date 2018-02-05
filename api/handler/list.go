package handler

import (
	"net/http"

	"github.com/rawmind0/norman/parse"
	"github.com/rawmind0/norman/types"
)

func ListHandler(request *types.APIContext, next types.RequestHandler) error {
	var (
		err  error
		data interface{}
	)

	store := request.Schema.Store
	if store == nil {
		return nil
	}

	if request.ID == "" {
		opts := parse.QueryOptions(request, request.Schema)
		data, err = store.List(request, request.Schema, &opts)
	} else if request.Link == "" {
		data, err = store.ByID(request, request.Schema, request.ID)
	} else {
		return request.Schema.LinkHandler(request, nil)
	}

	if err != nil {
		return err
	}

	request.WriteResponse(http.StatusOK, data)
	return nil
}
