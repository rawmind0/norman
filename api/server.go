package api

import (
	"net/http"

	"sync"

	"github.com/rawmind0/norman/api/access"
	"github.com/rawmind0/norman/api/builtin"
	"github.com/rawmind0/norman/api/handler"
	"github.com/rawmind0/norman/api/writer"
	"github.com/rawmind0/norman/authorization"
	"github.com/rawmind0/norman/httperror"
	"github.com/rawmind0/norman/parse"
	"github.com/rawmind0/norman/store/wrapper"
	"github.com/rawmind0/norman/types"
)

type StoreWrapper func(types.Store) types.Store

type Parser func(rw http.ResponseWriter, req *http.Request) (*types.APIContext, error)

type Server struct {
	initBuiltin                 sync.Once
	IgnoreBuiltin               bool
	Parser                      Parser
	Resolver                    parse.ResolverFunc
	SubContextAttributeProvider types.SubContextAttributeProvider
	ResponseWriters             map[string]ResponseWriter
	Schemas                     *types.Schemas
	QueryFilter                 types.QueryFilter
	StoreWrapper                StoreWrapper
	URLParser                   parse.URLParser
	Defaults                    Defaults
	AccessControl               types.AccessControl
}

type Defaults struct {
	ActionHandler types.ActionHandler
	ListHandler   types.RequestHandler
	LinkHandler   types.RequestHandler
	CreateHandler types.RequestHandler
	DeleteHandler types.RequestHandler
	UpdateHandler types.RequestHandler
	Store         types.Store
	ErrorHandler  types.ErrorHandler
}

func NewAPIServer() *Server {
	s := &Server{
		Schemas: types.NewSchemas(),
		ResponseWriters: map[string]ResponseWriter{
			"json": &writer.JSONResponseWriter{},
			"html": &writer.HTMLResponseWriter{},
		},
		SubContextAttributeProvider: &parse.DefaultSubContextAttributeProvider{},
		Resolver:                    parse.DefaultResolver,
		AccessControl:               &authorization.AllAccess{},
		Defaults: Defaults{
			CreateHandler: handler.CreateHandler,
			DeleteHandler: handler.DeleteHandler,
			UpdateHandler: handler.UpdateHandler,
			ListHandler:   handler.ListHandler,
			LinkHandler: func(*types.APIContext, types.RequestHandler) error {
				return httperror.NewAPIError(httperror.NotFound, "Link not found")
			},
			ErrorHandler: httperror.ErrorHandler,
		},
		StoreWrapper: wrapper.Wrap,
		URLParser:    parse.DefaultURLParser,
		QueryFilter:  handler.QueryFilter,
	}

	s.Schemas.AddHook = s.setupDefaults
	s.Parser = s.parser
	return s
}

func (s *Server) parser(rw http.ResponseWriter, req *http.Request) (*types.APIContext, error) {
	ctx, err := parse.Parse(rw, req, s.Schemas, s.URLParser, s.Resolver)
	ctx.ResponseWriter = s.ResponseWriters[ctx.ResponseFormat]
	if ctx.ResponseWriter == nil {
		ctx.ResponseWriter = s.ResponseWriters["json"]
	}

	if ctx.QueryFilter == nil {
		ctx.QueryFilter = s.QueryFilter
	}

	if ctx.SubContextAttributeProvider == nil {
		ctx.SubContextAttributeProvider = s.SubContextAttributeProvider
	}

	ctx.AccessControl = s.AccessControl

	return ctx, err
}

func (s *Server) AddSchemas(schemas *types.Schemas) error {
	if schemas.Err() != nil {
		return schemas.Err()
	}

	s.initBuiltin.Do(func() {
		if s.IgnoreBuiltin {
			return
		}
		for _, schema := range builtin.Schemas.Schemas() {
			s.Schemas.AddSchema(*schema)
		}
	})

	for _, schema := range schemas.Schemas() {
		s.Schemas.AddSchema(*schema)
	}

	return s.Schemas.Err()
}

func (s *Server) setupDefaults(schema *types.Schema) {
	if schema.ActionHandler == nil {
		schema.ActionHandler = s.Defaults.ActionHandler
	}

	if schema.Store == nil {
		schema.Store = s.Defaults.Store
	}

	if schema.ListHandler == nil {
		schema.ListHandler = s.Defaults.ListHandler
	}

	if schema.LinkHandler == nil {
		schema.LinkHandler = s.Defaults.LinkHandler
	}

	if schema.CreateHandler == nil {
		schema.CreateHandler = s.Defaults.CreateHandler
	}

	if schema.UpdateHandler == nil {
		schema.UpdateHandler = s.Defaults.UpdateHandler
	}

	if schema.DeleteHandler == nil {
		schema.DeleteHandler = s.Defaults.DeleteHandler
	}

	if schema.ErrorHandler == nil {
		schema.ErrorHandler = s.Defaults.ErrorHandler
	}

	if schema.Store != nil && s.StoreWrapper != nil {
		schema.Store = s.StoreWrapper(schema.Store)
	}
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if apiResponse, err := s.handle(rw, req); err != nil {
		s.handleError(apiResponse, err)
	}
}

func (s *Server) handle(rw http.ResponseWriter, req *http.Request) (*types.APIContext, error) {
	apiRequest, err := s.Parser(rw, req)
	if err != nil {
		return apiRequest, err
	}

	if err := CheckCSRF(apiRequest); err != nil {
		return apiRequest, err
	}

	action, err := ValidateAction(apiRequest)
	if err != nil {
		return apiRequest, err
	}

	if apiRequest.Schema == nil {
		return apiRequest, nil
	}

	if action == nil && apiRequest.Type != "" {
		var handler types.RequestHandler
		var nextHandler types.RequestHandler
		if apiRequest.Link == "" {
			switch apiRequest.Method {
			case http.MethodGet:
				if apiRequest.ID == "" {
					if !apiRequest.AccessControl.CanList(apiRequest, apiRequest.Schema) {
						return apiRequest, httperror.NewAPIError(httperror.PermissionDenied, "Can not list "+apiRequest.Schema.ID)
					}
				} else {
					if !apiRequest.AccessControl.CanGet(apiRequest, apiRequest.Schema) {
						return apiRequest, httperror.NewAPIError(httperror.PermissionDenied, "Can not get "+apiRequest.Schema.ID)
					}
				}
				handler = apiRequest.Schema.ListHandler
				nextHandler = s.Defaults.ListHandler
			case http.MethodPost:
				if !apiRequest.AccessControl.CanCreate(apiRequest, apiRequest.Schema) {
					return apiRequest, httperror.NewAPIError(httperror.PermissionDenied, "Can not create "+apiRequest.Schema.ID)
				}
				handler = apiRequest.Schema.CreateHandler
				nextHandler = s.Defaults.CreateHandler
			case http.MethodPut:
				if !apiRequest.AccessControl.CanUpdate(apiRequest, nil, apiRequest.Schema) {
					return apiRequest, httperror.NewAPIError(httperror.PermissionDenied, "Can not update "+apiRequest.Schema.ID)
				}
				handler = apiRequest.Schema.UpdateHandler
				nextHandler = s.Defaults.UpdateHandler
			case http.MethodDelete:
				if !apiRequest.AccessControl.CanDelete(apiRequest, nil, apiRequest.Schema) {
					return apiRequest, httperror.NewAPIError(httperror.PermissionDenied, "Can not delete "+apiRequest.Schema.ID)
				}
				handler = apiRequest.Schema.DeleteHandler
				nextHandler = s.Defaults.DeleteHandler
			}
		} else {
			handler = apiRequest.Schema.ListHandler
			nextHandler = s.Defaults.ListHandler
		}

		if handler == nil {
			return apiRequest, httperror.NewAPIError(httperror.NotFound, "")
		}

		return apiRequest, handler(apiRequest, nextHandler)
	} else if action != nil {
		return apiRequest, handleAction(action, apiRequest)
	}

	return apiRequest, nil
}

func handleAction(action *types.Action, context *types.APIContext) error {
	if context.ID != "" {
		if err := access.ByID(context, context.Version, context.Type, context.ID, nil); err != nil {
			return err
		}
	}
	return context.Schema.ActionHandler(context.Action, action, context)
}

func (s *Server) handleError(apiRequest *types.APIContext, err error) {
	if apiRequest.Schema == nil {
		s.Defaults.ErrorHandler(apiRequest, err)
	} else if apiRequest.Schema.ErrorHandler != nil {
		apiRequest.Schema.ErrorHandler(apiRequest, err)
	}
}
