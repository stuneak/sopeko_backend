CREATE TABLE ticker_prices (
  id           BIGSERIAL PRIMARY KEY,
  ticker_id    BIGINT NOT NULL REFERENCES tickers(id),
  price        NUMERIC(18,2) NOT NULL,
  recorded_at  TIMESTAMPTZ NOT NULL
);

-- one price per ticker per timestamp
CREATE UNIQUE INDEX uq_ticker_prices_ticker_time
  ON ticker_prices (ticker_id, recorded_at);

-- fast "latest price" queries
CREATE INDEX idx_ticker_prices_latest
  ON ticker_prices (ticker_id, recorded_at DESC);
