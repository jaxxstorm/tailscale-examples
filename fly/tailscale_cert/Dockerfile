FROM node:16.19.0-alpine

# Create app directory
WORKDIR /usr/src/app

# Install app dependencies
# A wildcard is used to ensure both package.json AND package-lock.json are copied
# where available (npm@5+)
COPY package*.json ./

# Install system dependencies
RUN apk update && apk add ca-certificates iptables ip6tables jq curl wget dumb-init && rm -rf /var/cache/apk/*

RUN npm install
# If you are building your code for production
# RUN npm ci --only=production

# Bundle app source
COPY . .

# Make start script executable
RUN chmod +x /usr/src/app/start.sh

# Copy Tailscale binaries from the tailscale image on Docker Hub.
COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscaled /app/tailscaled
COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscale /app/tailscale
RUN mkdir -p /var/run/tailscale /var/cache/tailscale /var/lib/tailscale

EXPOSE 8080 443

# Use dumb-init as init
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/src/app/start.sh"]
