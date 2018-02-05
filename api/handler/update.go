package handler

import (
	"net/http"

	"github.com/rawmind0/norman/types"
)

func UpdateHandler(apiContext *types.APIContext, next types.RequestHandler) error {
	data, err := ParseAndValidateBody(apiContext, false)
	if err != nil {
		return err
	}

	store := apiContext.Schema.Store
	if store != nil {
		data, err = store.Update(apiContext, apiContext.Schema, data, apiContext.ID)
		if err != nil {
			return err
		}
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}
