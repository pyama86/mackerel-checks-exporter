FROM golang:latest as builder
ADD . /opt/mackerel-checks-exporter
WORKDIR /opt/mackerel-checks-exporter/
RUN GOOS=linux make build

FROM scratch
COPY --from=builder /opt/mackerel-checks-exporter/tmp/bin/mackerel-checks-exporter /bin/mackerel-checks-exporter
EXPOSE 1104
CMD /bin/mackerel-checks-exporter --web.listen-address=0.0.0.0:9111 --mackerel.config-path=/opt/mackerel-checks-exporter.conf
