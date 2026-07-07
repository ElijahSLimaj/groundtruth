FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY services/serving/package.json services/serving/
COPY packages/shared/package.json packages/shared/
RUN pnpm install --frozen-lockfile --filter @company-brain/serving...
COPY packages/shared packages/shared
COPY services/serving services/serving
RUN pnpm --filter @company-brain/serving build

FROM node:22-alpine
ENV NODE_ENV=production
WORKDIR /repo
COPY --from=build /repo/node_modules node_modules
COPY --from=build /repo/services/serving/node_modules services/serving/node_modules
COPY --from=build /repo/services/serving/dist services/serving/dist
COPY --from=build /repo/services/serving/package.json services/serving/
USER node
CMD ["node", "services/serving/dist/main.js"]
