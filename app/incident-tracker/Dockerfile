FROM node:22-alpine
WORKDIR /app
COPY package.json ./
COPY src ./src
COPY public ./public
ENV NODE_ENV=production PORT=8080 DATA_FILE=/data/incidents.json
USER node
EXPOSE 8080
CMD ["node", "src/server.js"]
