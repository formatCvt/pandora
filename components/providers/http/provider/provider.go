package provider

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/yandex/pandora/components/providers/base"
	"github.com/yandex/pandora/components/providers/http/config"
	"github.com/yandex/pandora/components/providers/http/decoders"
	"github.com/yandex/pandora/core"
	"github.com/yandex/pandora/lib/confutil"
	"golang.org/x/xerrors"
)

type Provider struct {
	base.ProviderBase
	config.Config
	decoders.Decoder

	Close func() error

	AmmoPool sync.Pool
	Sink     chan *base.Ammo[http.Request]
}

func (p *Provider) Acquire() (core.Ammo, bool) {
	ammo, ok := <-p.Sink
	if ok {
		ammo.SetID(p.NextID())
	}
	return ammo, ok
}

func (p *Provider) Release(a core.Ammo) {
	ammo := a.(*base.Ammo[http.Request])
	// TODO: add request release for example for future fasthttp
	// ammo.Req.Body = nil
	ammo.Req = nil
	p.AmmoPool.Put(ammo)
}

func (p *Provider) Run(ctx context.Context, deps core.ProviderDeps) (err error) {
	var req *http.Request
	var tag string

	p.ProviderDeps = deps
	// defer close(p.Sink)
	defer func() {
		// TODO: wrap in go 1.20
		// err = errors.Join(err, p.Close())
		closeErr := p.Close()
		if closeErr != nil {
			if err != nil {
				err = xerrors.Errorf("Multiple errors faced: %w, %w", err, closeErr)
			} else {
				err = closeErr
			}
		}
	}()

	for {
		for p.Decoder.Scan(ctx) {
			req, tag = p.Decoder.Next()
			if !confutil.IsChosenCase(tag, p.Config.ChosenCases) {
				continue
			}
			a := p.AmmoPool.Get().(*base.Ammo[http.Request])
			a.Reset(req, tag)
			select {
			case <-ctx.Done():
				err = ctx.Err()
				if err != nil && !errors.Is(err, context.Canceled) {
					err = xerrors.Errorf("error from context: %w", err)
				}
			case p.Sink <- a:
			}
		}
		err = p.Decoder.Err()
		if err != nil {
			if errors.Is(err, decoders.ErrAmmoLimit) || errors.Is(err, decoders.ErrPassLimit) {
				err = nil
			}
			break
		}

		select {
		case <-ctx.Done():
			err = ctx.Err()
			if err != nil && !errors.Is(err, context.Canceled) {
				err = xerrors.Errorf("error from context: %w", err)
			}
		default:
		}
		if err != nil {
			break
		}
	}
	// named empty return for future wrapping provider.Close error
	return
}

var _ core.Provider = (*Provider)(nil)
