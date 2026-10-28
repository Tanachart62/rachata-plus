FROM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS build
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
ENV VITE_AUTH_MODE=api
RUN npm run build

FROM nginx:stable-alpine@sha256:47928bf67eb62a1863cb08d9ecb52d3e704304ba614eaee17884204677702951
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
COPY --chmod=0644 nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /web/dist /usr/share/nginx/html
RUN chmod -R a+rX /usr/share/nginx/html
USER 10001:10001
RUN nginx -t
EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
