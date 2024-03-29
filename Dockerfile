FROM ubuntu:jammy
ENV VERSION 0.2.3
RUN apt-get update -qqy && apt install -qqy wget sudo gnupg2
RUN echo "deb [arch=amd64] http://apt.mackerel.io/v2/ mackerel contrib" > /etc/apt/sources.list.d/mackerel.list
RUN wget -q -O - https://mackerel.io/file/cert/GPG-KEY-mackerel-v2 | apt-key add - && apt-get update -qqy && apt install -qqy mackerel-check-plugins

RUN wget https://github.com/pyama86/mackerel-checks-exporter/releases/download/v${VERSION}/mackerel-checks-exporter_${VERSION}-1_amd64.deb  && \
dpkg -i mackerel-checks-exporter_${VERSION}-1_amd64.deb && rm  -f mackerel-checks-exporter_${VERSION}-1_amd64.deb
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

CMD /usr/bin/mackerel-checks-exporter --web.listen-address=0.0.0.0:9111 --mackerel.config-path=/opt/mackerel-checks-exporter.conf
