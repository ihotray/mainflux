// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	kitot "github.com/go-kit/kit/tracing/opentracing"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/go-zoo/bone"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/internal/httputil"
	"github.com/mainflux/mainflux/pkg/errors"
	"github.com/mainflux/mainflux/pkg/uuid"
	"github.com/mainflux/mainflux/things"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	contentType = "application/json"
	offsetKey   = "offset"
	limitKey    = "limit"
	nameKey     = "name"
	orderKey    = "order"
	dirKey      = "dir"
	metadataKey = "metadata"
	disconnKey  = "disconnected"
	sharedKey   = "shared"
	defOffset   = 0
	defLimit    = 10
)

// MakeHandler returns a HTTP handler for API endpoints.
func MakeHandler(tracer opentracing.Tracer, svc things.Service) http.Handler {
	opts := []kithttp.ServerOption{
		kithttp.ServerErrorEncoder(encodeError),
	}

	r := bone.New()

	r.Post("/things", kithttp.NewServer(
		kitot.TraceServer(tracer, "create_thing")(createThingEndpoint(svc)),
		decodeThingCreation,
		encodeResponse,
		opts...,
	))

	r.Post("/things/bulk", kithttp.NewServer(
		kitot.TraceServer(tracer, "create_things")(createThingsEndpoint(svc)),
		decodeThingsCreation,
		encodeResponse,
		opts...,
	))

	r.Post("/things/:id/share", kithttp.NewServer(
		kitot.TraceServer(tracer, "share_thing")(shareThingEndpoint(svc)),
		decodeShareThing,
		encodeResponse,
		opts...,
	))

	r.Patch("/things/:id/key", kithttp.NewServer(
		kitot.TraceServer(tracer, "update_key")(updateKeyEndpoint(svc)),
		decodeKeyUpdate,
		encodeResponse,
		opts...,
	))

	r.Put("/things/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "update_thing")(updateThingEndpoint(svc)),
		decodeThingUpdate,
		encodeResponse,
		opts...,
	))

	r.Delete("/things/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "remove_thing")(removeThingEndpoint(svc)),
		decodeView,
		encodeResponse,
		opts...,
	))

	r.Get("/things/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "view_thing")(viewThingEndpoint(svc)),
		decodeView,
		encodeResponse,
		opts...,
	))

	r.Get("/things/:id/channels", kithttp.NewServer(
		kitot.TraceServer(tracer, "list_channels_by_thing")(listChannelsByThingEndpoint(svc)),
		decodeListByConnection,
		encodeResponse,
		opts...,
	))

	r.Get("/things", kithttp.NewServer(
		kitot.TraceServer(tracer, "list_things")(listThingsEndpoint(svc)),
		decodeList,
		encodeResponse,
		opts...,
	))

	r.Post("/things/search", kithttp.NewServer(
		kitot.TraceServer(tracer, "search_things")(listThingsEndpoint(svc)),
		decodeListByMetadata,
		encodeResponse,
		opts...,
	))

	r.Post("/channels", kithttp.NewServer(
		kitot.TraceServer(tracer, "create_channel")(createChannelEndpoint(svc)),
		decodeChannelCreation,
		encodeResponse,
		opts...,
	))

	r.Post("/channels/bulk", kithttp.NewServer(
		kitot.TraceServer(tracer, "create_channels")(createChannelsEndpoint(svc)),
		decodeChannelsCreation,
		encodeResponse,
		opts...,
	))

	r.Put("/channels/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "update_channel")(updateChannelEndpoint(svc)),
		decodeChannelUpdate,
		encodeResponse,
		opts...,
	))

	r.Delete("/channels/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "remove_channel")(removeChannelEndpoint(svc)),
		decodeView,
		encodeResponse,
		opts...,
	))

	r.Get("/channels/:id", kithttp.NewServer(
		kitot.TraceServer(tracer, "view_channel")(viewChannelEndpoint(svc)),
		decodeView,
		encodeResponse,
		opts...,
	))

	r.Get("/channels/:id/things", kithttp.NewServer(
		kitot.TraceServer(tracer, "list_things_by_channel")(listThingsByChannelEndpoint(svc)),
		decodeListByConnection,
		encodeResponse,
		opts...,
	))

	r.Get("/channels", kithttp.NewServer(
		kitot.TraceServer(tracer, "list_channels")(listChannelsEndpoint(svc)),
		decodeList,
		encodeResponse,
		opts...,
	))

	r.Post("/connect", kithttp.NewServer(
		kitot.TraceServer(tracer, "connect")(connectEndpoint(svc)),
		decodeConnectList,
		encodeResponse,
		opts...,
	))

	r.Put("/disconnect", kithttp.NewServer(
		kitot.TraceServer(tracer, "disconnect")(disconnectEndpoint(svc)),
		decodeConnectList,
		encodeResponse,
		opts...,
	))

	r.Put("/channels/:chanId/things/:thingId", kithttp.NewServer(
		kitot.TraceServer(tracer, "connect_thing")(connectThingEndpoint(svc)),
		decodeConnectThing,
		encodeResponse,
		opts...,
	))

	r.Delete("/channels/:chanId/things/:thingId", kithttp.NewServer(
		kitot.TraceServer(tracer, "disconnect_thing")(disconnectThingEndpoint(svc)),
		decodeConnectThing,
		encodeResponse,
		opts...,
	))

	r.Get("/groups/:groupId", kithttp.NewServer(
		kitot.TraceServer(tracer, "list_members")(listMembersEndpoint(svc)),
		decodeListMembersRequest,
		encodeResponse,
		opts...,
	))

	r.GetFunc("/health", mainflux.Health("things"))
	r.Handle("/metrics", promhttp.Handler())

	return r
}

func decodeThingCreation(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := createThingReq{token: t}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeThingsCreation(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := createThingsReq{token: t}
	if err := json.NewDecoder(r.Body).Decode(&req.Things); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeShareThing(ctx context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := shareThingReq{
		token:   t,
		thingID: bone.GetValue(r, "id"),
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeThingUpdate(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := updateThingReq{
		token: t,
		id:    bone.GetValue(r, "id"),
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeKeyUpdate(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := updateKeyReq{
		token: t,
		id:    bone.GetValue(r, "id"),
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeChannelCreation(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := createChannelReq{token: t}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeChannelsCreation(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := createChannelsReq{token: t}

	if err := json.NewDecoder(r.Body).Decode(&req.Channels); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeChannelUpdate(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := updateChannelReq{
		token: t,
		id:    bone.GetValue(r, "id"),
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeView(_ context.Context, r *http.Request) (interface{}, error) {
	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := viewResourceReq{
		token: t,
		id:    bone.GetValue(r, "id"),
	}

	return req, nil
}

func decodeList(_ context.Context, r *http.Request) (interface{}, error) {
	o, err := httputil.ReadUintQuery(r, offsetKey, defOffset)
	if err != nil {
		return nil, err
	}

	l, err := httputil.ReadUintQuery(r, limitKey, defLimit)
	if err != nil {
		return nil, err
	}

	n, err := httputil.ReadStringQuery(r, nameKey, "")
	if err != nil {
		return nil, err
	}

	or, err := httputil.ReadStringQuery(r, orderKey, "")
	if err != nil {
		return nil, err
	}

	d, err := httputil.ReadStringQuery(r, dirKey, "")
	if err != nil {
		return nil, err
	}

	m, err := httputil.ReadMetadataQuery(r, metadataKey, nil)
	if err != nil {
		return nil, err
	}
	shared, err := httputil.ReadBoolQuery(r, sharedKey, false)
	if err != nil {
		return nil, err
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := listResourcesReq{
		token: t,
		pageMetadata: things.PageMetadata{
			Offset:            o,
			Limit:             l,
			Name:              n,
			Order:             or,
			Dir:               d,
			Metadata:          m,
			FetchSharedThings: shared,
		},
	}

	return req, nil
}

func decodeListByMetadata(_ context.Context, r *http.Request) (interface{}, error) {
	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := listResourcesReq{token: t}
	if err := json.NewDecoder(r.Body).Decode(&req.pageMetadata); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeListByConnection(_ context.Context, r *http.Request) (interface{}, error) {
	o, err := httputil.ReadUintQuery(r, offsetKey, defOffset)
	if err != nil {
		return nil, err
	}

	l, err := httputil.ReadUintQuery(r, limitKey, defLimit)
	if err != nil {
		return nil, err
	}

	c, err := httputil.ReadBoolQuery(r, disconnKey, false)
	if err != nil {
		return nil, err
	}

	or, err := httputil.ReadStringQuery(r, orderKey, "")
	if err != nil {
		return nil, err
	}

	d, err := httputil.ReadStringQuery(r, dirKey, "")
	if err != nil {
		return nil, err
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := listByConnectionReq{
		token: t,
		id:    bone.GetValue(r, "id"),
		pageMetadata: things.PageMetadata{
			Offset:       o,
			Limit:        l,
			Disconnected: c,
			Order:        or,
			Dir:          d,
		},
	}

	return req, nil
}

func decodeConnectThing(_ context.Context, r *http.Request) (interface{}, error) {
	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := connectThingReq{
		token:   t,
		chanID:  bone.GetValue(r, "chanId"),
		thingID: bone.GetValue(r, "thingId"),
	}

	return req, nil
}

func decodeConnectList(_ context.Context, r *http.Request) (interface{}, error) {
	if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
		return nil, errors.ErrUnsupportedContentType
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := connectReq{token: t}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(errors.ErrMalformedEntity, err)
	}

	return req, nil
}

func decodeListMembersRequest(_ context.Context, r *http.Request) (interface{}, error) {
	o, err := httputil.ReadUintQuery(r, offsetKey, defOffset)
	if err != nil {
		return nil, err
	}

	l, err := httputil.ReadUintQuery(r, limitKey, defLimit)
	if err != nil {
		return nil, err
	}

	m, err := httputil.ReadMetadataQuery(r, metadataKey, nil)
	if err != nil {
		return nil, err
	}

	t, err := httputil.ExtractAuthToken(r)
	if err != nil {
		return nil, err
	}

	req := listThingsGroupReq{
		token:   t,
		groupID: bone.GetValue(r, "groupId"),
		pageMetadata: things.PageMetadata{
			Offset:   o,
			Limit:    l,
			Metadata: m,
		},
	}
	return req, nil
}

func encodeResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", contentType)

	if ar, ok := response.(mainflux.Response); ok {
		for k, v := range ar.Headers() {
			w.Header().Set(k, v)
		}

		w.WriteHeader(ar.Code())

		if ar.Empty() {
			return nil
		}
	}

	return json.NewEncoder(w).Encode(response)
}

func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	switch {
	case errors.Contains(err, errors.ErrAuthentication):
		w.WriteHeader(http.StatusUnauthorized)
	case errors.Contains(err, errors.ErrAuthorization):
		w.WriteHeader(http.StatusForbidden)
	case errors.Contains(err, errors.ErrUnsupportedContentType):
		w.WriteHeader(http.StatusUnsupportedMediaType)
	case errors.Contains(err, errors.ErrInvalidQueryParams),
		errors.Contains(err, errors.ErrMalformedEntity):
		w.WriteHeader(http.StatusBadRequest)
	case errors.Contains(err, errors.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case errors.Contains(err, errors.ErrConflict):
		w.WriteHeader(http.StatusConflict)
	case errors.Contains(err, errors.ErrScanMetadata):
		w.WriteHeader(http.StatusUnprocessableEntity)

	case errors.Contains(err, errors.ErrCreateEntity),
		errors.Contains(err, errors.ErrUpdateEntity),
		errors.Contains(err, errors.ErrViewEntity),
		errors.Contains(err, errors.ErrRemoveEntity):
		w.WriteHeader(http.StatusInternalServerError)

	case errors.Contains(err, uuid.ErrGeneratingID):
		w.WriteHeader(http.StatusInternalServerError)

	default:
		w.WriteHeader(http.StatusInternalServerError)
	}

	if errorVal, ok := err.(errors.Error); ok {
		w.Header().Set("Content-Type", contentType)
		if err := json.NewEncoder(w).Encode(httputil.ErrorRes{Err: errorVal.Msg()}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
