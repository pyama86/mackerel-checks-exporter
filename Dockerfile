FROM debian:buster-slim
COPY mackerel*.deb /tmp/
RUN dpkg -i /tmp/mackerel*.deb

EXPOSE 9111
CMD /usr/bin/mackerel-checks-exporter --web.listen-address=0.0.0.0:9111 --mackerel.config-path=/opt/mackerel-checks-exporter.conf
