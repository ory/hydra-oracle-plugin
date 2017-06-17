FROM cperfect/golang-ora

RUN go get github.com/Masterminds/glide
WORKDIR /go/src/github.com/ory/hydra-oracle-plugin

ADD ./glide.yaml ./glide.yaml
ADD ./glide.lock ./glide.lock

RUN glide install -v

ADD . .

RUN go install .

ENTRYPOINT /go/bin/hydra-oracle-plugin

EXPOSE 4040
