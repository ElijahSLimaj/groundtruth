FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/shared/package.json packages/shared/
RUN pnpm install --frozen-lockfile --filter @company-brain/web...
COPY packages/shared packages/shared
COPY apps/web apps/web
ARG NEXT_PUBLIC_SUPABASE_URL
ARG NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY
ENV NEXT_PUBLIC_SUPABASE_URL=$NEXT_PUBLIC_SUPABASE_URL
ENV NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY=$NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY
RUN pnpm --filter @company-brain/web build

FROM node:22-alpine
ENV NODE_ENV=production
WORKDIR /repo/apps/web
COPY --from=build /repo/node_modules /repo/node_modules
COPY --from=build /repo/apps/web/node_modules node_modules
COPY --from=build /repo/apps/web/.next .next
COPY --from=build /repo/apps/web/package.json ./
COPY --from=build /repo/apps/web/public public
USER node
CMD ["./node_modules/.bin/next", "start"]
