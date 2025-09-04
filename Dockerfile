FROM heroiclabs/nakama-pluginbuilder:3.30.0 AS builder

ENV GO111MODULE=on
ENV CGO_ENABLED=1

WORKDIR /nakama
COPY . .

RUN go build --trimpath --buildmode=plugin -o ./backend.so

FROM heroiclabs/nakama:3.30.0

COPY --from=builder /nakama/backend.so /nakama/data/modules/
COPY --from=builder /nakama/local.yml /nakama/data/
COPY --from=builder /nakama/*.json /nakama/data/modules/