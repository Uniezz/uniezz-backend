import express from 'express';

import { loadEnv } from '@/config/env';

export function createApp(): express.Express {
  const app = express();

  app.use(express.json());

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', service: 'uniezz-api' });
  });

  return app;
}

if (import.meta.main) {
  const env = loadEnv();
  const app = createApp();

  app.listen(env.PORT, () => {
    console.log(`uniezz-api listening on http://localhost:${env.PORT}`);
  });
}
