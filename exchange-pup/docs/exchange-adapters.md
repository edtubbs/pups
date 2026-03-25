# exchange adapters

Current adapters:
- Binance REST: ticker book + klines
- Kraken REST: ticker + OHLC

Both normalize into internal `MarketTick`, `OrderBook`, `Candle`.

Future work:
- WebSocket streams (Binance, Kraken v2)
- richer depth/trade normalization
