package host

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"

	"github.com/hansshaw/OpenNovel/assets"
	"github.com/hansshaw/OpenNovel/internal/bootstrap"
)

type blockedMetadataRequest struct {
	started chan *http.Request
	release chan struct{}
}

func (b blockedMetadataRequest) RoundTrip(r *http.Request) (*http.Response, error) {
	b.started <- r
	<-b.release
	return nil, context.Canceled
}

func TestHostCloseCancelsAndJoinsMetadataRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := blockedMetadataRequest{started: make(chan *http.Request, 1), release: make(chan struct{})}
		previous := http.DefaultTransport
		http.DefaultTransport = transport
		defer func() { http.DefaultTransport = previous }()

		cfg := bootstrap.Config{Provider: "openai", ModelName: "fixture", OutputDir: t.TempDir(),
			Providers: map[string]bootstrap.ProviderConfig{"openai": {APIKey: "stub"}}}
		h, err := New(cfg, assets.Bundle{}, WithUserDataDir(t.TempDir()))
		if err != nil {
			t.Fatal(err)
		}
		request := <-transport.started
		closed := make(chan struct{})
		go func() {
			h.Close()
			close(closed)
		}()

		// Leave the request blocked after cancellation, as a response or cache
		// writer can still be unwinding. Close must join the entire refresh.
		synctest.Wait()
		if request.Context().Err() != context.Canceled {
			t.Error("closing the host did not cancel the metadata request")
		}
		select {
		case <-closed:
			t.Error("host closed before the metadata refresh returned")
		default:
		}
		close(transport.release)
		<-closed
	})
}
