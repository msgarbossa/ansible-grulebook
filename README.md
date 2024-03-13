# ansible-grulebook

ansible-grulebook is a lightweight replacement for ansible-rulebook written in Go using the Grule rule engine

This is a work in progress.

Working:
1. Configuration file for listener (config.yml)
2. Listener can read alert messages from Prometheus Alertmanager
3. HTTP handler parses metadata from Alertmanager alert
4. Alert metadata is compared against rules defined using Grule (rules.grl)
5. Matching Grule rules (rules.grl) set Ansible-related facts to determine action (sets playbook and inventory)
6. Testing with `go test` executes all of the above steps.


Not yet working:
1. Does not trigger Ansible execution (will likely call [ansible-shim](https://github.com/msgarbossa/ansible-shim) through either a message queue or API call)


## Example

config.yml (configures HTTP listner):
```yaml
---
sources:
  - name: Listen for Alertmanager alerts
    type: alertmanager
    host: 0.0.0.0
    port: 5001
```

rules.grl (maps Alertmanager metadata to Ansible parameters):
```
rule HostOutOfDiskSpace "Check for disk space alert" salience 10 {
  when
    Fact.Status == "firing" && Fact.Labels["alertname"] == "HostOutOfDiskSpace"
  then
    Fact.Playbook = "check_filesystem.yml";
    Fact.InventoryFile = "localhost.yml";
    Retract("HostOutOfDiskSpace");
}
```

## Testing

```bash
task go_test
```

or

```bash
go test
```

## Manual Test

The curl-alertmanager-webhook.sh script can be used to send a simulated Prometheus Alertmanager alert with a current timestamp.

```bash
$ go run main.go&
[1] 383913
$ 2024/03/13 10:13:15 INFO server started: :5001
$ ./curl-alertmanager-webhook.sh
2024/03/13 10:13:20 INFO headers: map[Accept:[*/*] Content-Length:[3072] Content-Type:[application/json] User-Agent:[Alertmanager/0.26.1]]
2024/03/13 10:13:20 INFO HostOutOfDiskSpace, firing, map[alertname:HostOutOfDiskSpace app:demo1 device:/dev/mapper/app_vg-var_lib_docker_lv env:lab fstype:xfs instance:host1.acme.com:9100 job:node mountpoint:/var/lib/docker severity:warning]
2024/03/13 10:13:20 INFO check_filesystem.yml
2024/03/13 10:13:20 INFO localhost.yml
2024/03/13 10:13:20 INFO 
2024/03/13 10:13:20 INFO HostOutOfDiskSpace, firing, map[alertname:HostOutOfDiskSpace app:demo1 device:/dev/mapper/rootvg-optlv env:lab fstype:xfs instance:host1.acme.com:9100 job:node mountpoint:/opt severity:testing]
2024/03/13 10:13:20 INFO check_filesystem.yml
2024/03/13 10:13:20 INFO localhost.yml
2024/03/13 10:13:20 INFO 
2024/03/13 10:13:20 INFO HostOutOfDiskSpace, firing, map[alertname:HostOutOfDiskSpace app:demo1 device:/dev/sda2 env:lab fstype:xfs instance:host1.acme.com:9100 job:node mountpoint:/boot severity:testing]
2024/03/13 10:13:20 INFO check_filesystem.yml
2024/03/13 10:13:20 INFO localhost.yml
2024/03/13 10:13:20 INFO 
2024/03/13 10:13:20 INFO Finished processing webhook
$ fg
go run main.go
^CRunning cancel()
```
