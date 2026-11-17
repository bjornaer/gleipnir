-- Local models gleip can pick. size_gb = Ollama download size (≈ memory to load).
-- rank: lower = better for agentic coding. Hand-picked; refresh when new models land.
-- Sizes/licenses pulled from registry.ollama.ai manifests, 2026-11-17.
CREATE TABLE models (
  id      TEXT PRIMARY KEY, -- ollama tag
  size_gb REAL NOT NULL,
  license TEXT NOT NULL,
  rank    INTEGER NOT NULL UNIQUE
);

INSERT INTO models VALUES
  ('qwen3-coder:480b',               290.1, 'Apache-2.0',                 1),
  ('gpt-oss:120b',                    65.4, 'Apache-2.0',                 2),
  ('qwen3.8:27b',                     17.7, 'Apache-2.0',                 3),
  ('nemotron-3.5-lightning:30b-a3b',  25.4, 'NVIDIA Open Model License',  4),
  ('qwen3-coder:30b',                 18.6, 'Apache-2.0',                 5),
  ('devstral:24b',                    14.3, 'Apache-2.0',                 6),
  ('gpt-oss:20b',                     13.8, 'Apache-2.0',                 7),
  ('mimo-v2.6:9b',                     6.6, 'MIT',                        8),
  ('qwen3:14b',                        9.3, 'Apache-2.0',                 9),
  ('qwen3:8b',                         5.2, 'Apache-2.0',                10),
  ('qwen3:4b',                         2.5, 'Apache-2.0',                11),
  ('lfm3:1.7b',                        1.1, 'LFM Open License v1.0',     12);
