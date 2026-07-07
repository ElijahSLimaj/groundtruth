import { INestApplication, ValidationPipe } from '@nestjs/common';

export function configureApp(app: INestApplication): INestApplication {
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      forbidNonWhitelisted: true,
      errorHttpStatusCode: 422,
      transform: true,
    }),
  );
  return app;
}
