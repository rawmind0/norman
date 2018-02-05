package handler

import (
	"github.com/rawmind0/norman/parse"
	"github.com/rawmind0/norman/parse/builder"
	"github.com/rawmind0/norman/types"
)

func ParseAndValidateBody(apiContext *types.APIContext, create bool) (map[string]interface{}, error) {
	data, err := parse.Body(apiContext.Request)
	if err != nil {
		return nil, err
	}

	b := builder.NewBuilder(apiContext)

	op := builder.Create
	if !create {
		op = builder.Update
	}
	data, err = b.Construct(apiContext.Schema, data, op)
	if err != nil {
		return nil, err
	}

	return data, nil
}
