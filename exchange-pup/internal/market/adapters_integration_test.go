package market

import (
"context"
"net/http"
"net/http/httptest"
"testing"
"time"
)

func TestBinanceKrakenAdapterInterface(t *testing.T) {
_ = httptest.NewServer(http.NotFoundHandler())
client := NewClient()
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
_ = ctx
_ = client
_ = NewBinance(client)
_ = NewKraken(client)
}
