// Copyright (c) 2017 Yandex LLC. All rights reserved.
// Use of this source code is governed by a MPL 2.0
// license that can be found in the LICENSE file.
// Author: Vladimir Skipor <skipor@yandex-team.ru>

package phttp

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/facebookgo/stackerr"

	"github.com/yandex/pandora/aggregate"
	"github.com/yandex/pandora/ammo"
	"github.com/yandex/pandora/gun"
)

type Base struct {
	Do      func(r *http.Request) (*http.Response, error) // Required.
	Connect func(ctx context.Context) error               // Optional hook.
	Results chan<- *aggregate.Sample                      // Lazy set via BindResultTo.
}

var _ gun.Gun = (*Base)(nil)

func (b *Base) BindResultsTo(results chan<- *aggregate.Sample) {
	if b.Results != nil {
		log.Panic("already binded")
	}
	if results == nil {
		log.Panic("nil results")
	}
	b.Results = results
}

// Shoot to target, this method is not thread safe
func (b *Base) Shoot(ctx context.Context, a ammo.Ammo) (err error) {
	if b.Results == nil {
		log.Panic("must bind before shoot")
	}
	if b.Connect != nil {
		err = b.Connect(ctx)
		log.Printf("Connect error: %s\n", err)
		return
	}

	ha := a.(ammo.HTTP)
	req, sample := ha.Request()
	defer func() {
		if err != nil {
			sample.SetErr(err)
		}
		b.Results <- sample
		err = stackerr.WrapSkip(err, 1)
	}()
	var res *http.Response
	res, err = b.Do(req)
	if err != nil {
		log.Printf("Error performing a request: %s\n", err)
		return
	}
	defer res.Body.Close()
	// TODO: measure body read
	_, err = io.Copy(ioutil.Discard, res.Body)
	if err != nil {
		log.Printf("Error reading response body: %s\n", err)
		return
	}
	sample.SetProtoCode(res.StatusCode)
	// TODO: verbose logging
	return
}

func (b *Base) Close() {}
