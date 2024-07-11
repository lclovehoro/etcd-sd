# etcd-sd
prometheus custom service discovery with etcd


etcd数据格式：
'''
/services/<service_name>/<instance_id> = <instance_address>
'''

./etcd_sd -target-file xxx/xxx.json

数据格式如下：
'''
[{"targets":["10.10.12.1","10.10.12.2","10.10.12.3"],"labels":{"job":"kafka"}},{"targets":["10.10.10.1","10.10.10.2"],"labels":{"job":"nodes"}},{"targets":["10.10.11.1"],"labels":{"job":"redis"}}]
'''

prometheus.yaml:
'''
......
  - job_name: 'discovery_by_etcd' # Will be overwritten by job label of target groups.
    file_sd_configs:
    - files: ['/targets/*.json']
'''
