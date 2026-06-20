package appstore

const openpanelClickhouseConfigXML = `<clickhouse>
  <logger>
    <level>warning</level>
    <console>true</console>
  </logger>
  <keep_alive_timeout>10</keep_alive_timeout>
  <query_thread_log remove="remove"/>
  <query_log remove="remove"/>
  <text_log remove="remove"/>
  <trace_log remove="remove"/>
  <metric_log remove="remove"/>
  <asynchronous_metric_log remove="remove"/>
  <session_log remove="remove"/>
  <part_log remove="remove"/>
  <listen_host>0.0.0.0</listen_host>
  <interserver_listen_host>0.0.0.0</interserver_listen_host>
  <interserver_http_host>op-ch</interserver_http_host>
  <zookeeper>
    <node>
      <host>localhost</host>
      <port>9181</port>
    </node>
  </zookeeper>
  <remote_servers>
    <openpanel_cluster>
      <shard>
        <replica>
          <host>op-ch</host>
          <port>9000</port>
        </replica>
      </shard>
    </openpanel_cluster>
  </remote_servers>
</clickhouse>
`

const openpanelClickhouseUserConfigXML = `<clickhouse>
  <profiles>
    <default>
      <log_queries>0</log_queries>
      <log_query_threads>0</log_query_threads>
    </default>
  </profiles>
</clickhouse>
`

const openpanelClickhouseInitDB = `#!/bin/bash
set -e
clickhouse client -n <<-EOSQL
  CREATE DATABASE IF NOT EXISTS openpanel;
EOSQL
`
