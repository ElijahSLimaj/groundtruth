import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { loadConfig } from './config';
import { configureApp } from './configure-app';

async function bootstrap(): Promise<void> {
  const config = loadConfig();
  const app = configureApp(
    await NestFactory.create(AppModule, { rawBody: true }),
  );
  await app.listen(config.port);
}

bootstrap().catch((error: unknown) => {
  console.error(error);
  process.exit(1);
});
