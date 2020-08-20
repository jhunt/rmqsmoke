#!/bin/bash

docker exec -it cluster_rmq1_1 rabbitmqctl set_operator_policy ops-ha "." '{"ha-mode":"exactly","ha-params":2}' --apply-to queues
docker exec -it cluster_rmq1_1 rabbitmqctl list_operator_policies
