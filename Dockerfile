# To build:
# $ docker run --rm -v $(pwd):/go/src/github.com/sozercan/k8s-oidc-helper-azure -w /go/src/github.com/sozercan/k8s-oidc-helper-azure golang:1.7  go build -v -a -tags netgo -installsuffix netgo -ldflags '-w'
# $ docker build -t sozercan/k8s-oidc-helper-azure .
#
# To run:
# $ docker run sozercan/k8s-oidc-helper-azure

FROM busybox

MAINTAINER Sertac Ozercan, <sozercan@gmail.com>

COPY k8s-oidc-helper-azure /bin/k8s-oidc-helper-azure
RUN chmod 755 /bin/k8s-oidc-helper-azure

ENTRYPOINT ["/bin/k8s-oidc-helper-azure"]
