# mackerel-checks-exporter
this is migration tool mackerel to prometeus.

```bash
make
./mackrel-checks-exporter [flags]
```

## usage

```
usage: mackerel-checks-exporter [<flags>]

Flags:
  -h, --help               Show context-sensitive help (also try --help-long and --help-man).
      --web.listen-address=":9111"
                           Address to listen on for web interface and telemetry.
      --web.telemetry-path="/metrics"
                           Path under which to expose metrics.
      --mackerel.config-path="/etc/mackerel-agent/mackerel-agent.conf"
                           Mackerel Config Path.
      --log.level=info     Only log messages with the given severity or above. One of: [debug, info, warn, error]
      --log.format=logfmt  Output format of log messages. One of: [logfmt, json]
```

## author
- @pyama86
