FROM node:24-alpine AS build
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
ENV VITE_AUTH_MODE=api
RUN npm run build

FROM nginx:stable-alpine
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /web/dist /usr/share/nginx/html
USER 10001:10001
RUN nginx -t
EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
