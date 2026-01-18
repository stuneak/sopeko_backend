
CREATE TABLE tickers (
  id            BIGSERIAL PRIMARY KEY,
  symbol        TEXT NOT NULL UNIQUE,
  company_name  TEXT NOT NULL,
  exchange      TEXT NOT NULL, -- "NASDAQ" 
  currency      TEXT NOT NULL DEFAULT 'USD',
  created_at    TIMESTAMP NOT NULL DEFAULT now()
);
