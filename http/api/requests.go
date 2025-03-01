// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/mainflux/mainflux/pkg/errors"
	"github.com/mainflux/mainflux/pkg/messaging"
)

type publishReq struct {
	msg   messaging.Message
	token string
}

func (req publishReq) validate() error {
	if req.token == "" {
		return errors.ErrAuthentication
	}

	return nil
}
