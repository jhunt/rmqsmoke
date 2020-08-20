# RabbitMQ Cluster Migrations

> This write-up is an attempt to document the background, experimental methodology, and ultimate implementation of migrating clusters from one set of machines to another, with no downtime for durable exchanges and HA queues.

## The Docker Laboratory

For running experiments, and validating both the feasibility and fitness of any migration path we pursue, I set up a small lab, using Docker Compose.  This lab is indeed small — it can run comfortably on a decently-appointed laptop, or cloud virtual machine.

The Compose recipe consists of 7 services - a single HAproxy instance load balancing the remaining six RabbitMQ instances.  The load balancer is statically configured to round-robin traffic between "alive" instances of the Rabbit nodes.  This will allow us to have epmd (the Erlang runtime that RabbitMQ sits atop) up and functioning, for node maintenance, without starting the `rabbit` application and thereby partaking in cluster operations (until we are ready!)

Each RabbitMQ instance has been configured with a persistent, bind-mounted data directory (`/var/lib/rabbitmq`).  Primarily, we used this to investigate the on-disk data and "restart" failed configurations for troubleshooting / diagnostic purposes.  However, it turned out to be _essential_, since the data directory is the only place to mount in the _Erlang Cookie_, a synchronization secret used to ensure that all cluster members were aligned properly.

To treat this directory as "mostly ephemeral", we introduced a local shell script, `./run`, that would rebuild pristine data directories with the Erlang Cookie file properly created / populated / chowned.

## The Procedure

The migration procedure is this:

1. Start with a 3-node cluster, behind a single load balancer IP
2. Introduce 3 additional nodes (the "new" cluster)
3. Decommission the original 3 nodes

We rely on the durability of queues and exchanges, and queue mirroring / raft+quorum semantics to weather the removal of the original nodes from the cluster in step 3.

In particular this means:

- Non-durable queues / exchanges (including exclusive queues) _will go away during step 3.
- Non-mirrored classic queues, even those with persistent messages, may lose messages

At first blush, these seem problematic, especially for a planned migration.  However, upon further scrutiny, we find that applications relying on non-durable, non-HA queues are susceptible to the same problems during _unplanned_ outages.  This is okay — it means that the application owner has either de-prioritized survivability, or has chosen to implement it elsewhere.  Both of these are the knowing and explicit choice of the RabbitMQ customer.

Indeed, our chosen migration path is really nothing more than a controlled and orderly outage, where we get to bring up the replacement nodes **first**.

## Spinning up Docker Containers

To start the lab:

    $ ./run

Congratulations, you have 7 new containers, and a screen full of useful log messages about each.

    $ docker ps | grep rmqlab
    5c53df55ebcf        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq6_1
    e07a3d287f4c        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq3_1
    0f0fa0aa83c0        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq5_1
    bbb9fce693e9        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq2_1
    a5613bb83d0a        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq4_1
    2df6d6c9b4d7        rmqlab_lb           "/docker-entrypoint.…"   About a minute ago   Up About a minute   0.0.0.0:1936->1936/tcp, 0.0.0.0:25672->5672/tcp   rmqlab_lb_1
    58d8e93b38b8        rabbitmq:3.8.6      "docker-entrypoint.s…"   About a minute ago   Up About a minute   4369/tcp, 5671-5672/tcp, 25672/tcp                rmqlab_rmq1_1

While the HTTP Management Plugin configuration is set in the provided `rabbitmq.conf` file, we still need to activate the plugin on all of our instances.  We can do this with a for loop:

    for x in {1..6}; do
      docker exec -it rmqlab_rmq${x}_1 rabbitmq-plugins \
        enable rabbitmq_management
    done

To check on the health of each RabbitMQ instance, you can watch the hatop console for the load balancer in another terminal:

    $ ./lab hatop

Once all of these backends show as `UP`, we are ready to move onto the cluster setup.

## Setting up the Initial Cluster

We start with a 3-node cluster.

Out of the gate, each `rmqlab_rmqX_1` container is an island - a solitary, single-node cluster with no care or concern for any of ther other containers.  We can validate this by checking the output of a `rabbitmqctl cluster_status` on any given node:

    $ docker exec -it rmqlab_rmq3_1 rabbitmqctl cluster_status
    Cluster status of node rabbit@rmq3 ...
    Basics

    Cluster name: rabbit@rmq3

    Disk Nodes

    rabbit@rmq3

    Running Nodes

    rabbit@rmq3

    Versions

    rabbit@rmq3: RabbitMQ 3.8.6 on Erlang 23.0.3

    Alarms

    (none)

    Network Partitions

    (none)

    Listeners

    Node: rabbit@rmq3, interface: [::], port: 25672, protocol: clustering, purpose: inter-node and CLI tool communication
    Node: rabbit@rmq3, interface: [::], port: 5672, protocol: amqp, purpose: AMQP 0-9-1 and AMQP 1.0

    Feature flags

    Flag: implicit_default_bindings, state: enabled
    Flag: quorum_queue, state: enabled
    Flag: virtual_host_metadata, state: enabled

The other five nodes will look similar.

We'll start by joining rmq2 and rmq3 to rmq1, to form a 3-node cluster:

    for x in 2 3; do
      docker exec -it rmqlab_rmq${x}_1 rabbitmqctl stop_app
      docker exec -it rmqlab_rmq${x}_1 rabbitmqctl reset
      docker exec -it rmqlab_rmq${x}_1 rabbitmqctl join_cluster rabbit@rmq1
      docker exec -it rmqlab_rmq${x}_1 rabbitmqctl start_app
    done
    
    Stopping rabbit application on node rabbit@rmq2 ...
    Resetting node rabbit@rmq2 ...
    Clustering node rabbit@rmq2 with rabbit@rmq1
    Starting node rabbit@rmq2 ...
    Stopping rabbit application on node rabbit@rmq3 ...
    Resetting node rabbit@rmq3 ...
    Clustering node rabbit@rmq3 with rabbit@rmq1
    Starting node rabbit@rmq3 ...

Once that is complete, you can check cluster_status again on nodes 1, 2, or 3:

    $ docker exec -it rmqlab_rmq3_1 rabbitmqctl cluster_status | grep Node:
    Node: rabbit@rmq1, interface: [::], port: 25672, protocol: clustering, purpose: inter-node and CLI tool communication
    Node: rabbit@rmq1, interface: [::], port: 5672, protocol: amqp, purpose: AMQP 0-9-1 and AMQP 1.0
    Node: rabbit@rmq2, interface: [::], port: 25672, protocol: clustering, purpose: inter-node and CLI tool communication
    Node: rabbit@rmq2, interface: [::], port: 5672, protocol: amqp, purpose: AMQP 0-9-1 and AMQP 1.0
    Node: rabbit@rmq3, interface: [::], port: 25672, protocol: clustering, purpose: inter-node and CLI tool communication
    Node: rabbit@rmq3, interface: [::], port: 5672, protocol: amqp, purpose: AMQP 0-9-1 and AMQP 1.0
    
Voila! A three-node RMQ cluster.

The last thing we need to do is stop the `rabbit` application on the other three instances (4-6) so that the load balancer doesn't send **any** traffic to them.  That would be some serious split-brain.

    for x in {4..6}; do
      docker exec -it rmqlab_rmq${x}_1 rabbitmqctl stop_app
    done

We can check hatop now — the first three backends should be `UP` and the last three should be `DOWN`.

    $ docker exec -it rmqlab_lb_1 hatop -s /var/run/haproxy.stats

## Running App(s) Against the Cluster

With the cluster up and spinning, it's time to start setting up applications to do their work through the cluster messaging backplane.

For this experiment, we want an application that is simple but not too simple.  Primarily, it needs to (a) generate a large enough message load and (b) can be verified "sight unseen".  To meet these two criteria, we wrote a custom application that sends numbered messages that look like this:

    Mon Jan 2 15:04:05 -0700 MST 2006|0|978

Each message is a pipe-delimited triple of a timestamp, a batch counter, and a message number.  In the above example, the batch is `0` and the message number is `978`.  Message numbers range from 0 to 9999, and when they roll over, the batch counter is incremented.

The receiver then consumes a queue to receive these messages, and keeps track of how much of each batch has been received.  Every time a new batch is detected, the consumer prints out a summary of which batches are complete, and which ones still have outstanding messages.  The output looks like this:

    2020/08/19 19:45:55 --[ rx.START    36   0% ]----------
    2020/08/19 19:45:55 batch 0 ... 35 COMPLETE
    2020/08/19 19:45:55 batch 36 IN PROGRESS
    2020/08/19 19:46:01 --[ rx.COMPLETE 36 100% ]----------
    2020/08/19 19:46:01 --[ rx.START    37   0% ]----------
    2020/08/19 19:46:01 batch 0 ... 36 COMPLETE
    2020/08/19 19:46:01 batch 37 IN PROGRESS

This is from a clean, happy run-through where the receiver saw every message it expected to see in the first 37 batches (0 - 36), and the last batch is still in-progress.

To get this machine up and running, with our cluster spinning, we need to first initialize the exchange / queue in RabbitMQ:

    $ ./rmqsmoke init

This sets up a durable exchange named `rmq-default`, routing to a classic, mirrored queue named `rmq-default`.  The queue has 1 replica (in addition to the leader node).

Then, we start the consumer (`rx`) and producer (`tx`), each in its own terminal:

    $ ./rmqsmoke rx
    $ ./rmqsmoke tx

## Testing HA  / Injecting Failure

With our application running, we can test the survivability of our RabbitMQ cluster (and the applications usage _of_ the broker) by taking down individual parts of the cluster, in a controlled fashion.

Before we begin, let's look at the queue, to see which nodes the leader and mirror replica ended up on:

    $ ./lab qstat 1
    Timeout: 60.0 seconds ...
    Listing queues for vhost / ...
    name    state   pid     slave_pids      synchronised_slave_pids
    rmq-default     running <rabbit@rmq2.1597879162.3502.1> [<rabbit@rmq1.1597879162.3575.1>]       [<rabbit@rmq1.1597879162.3575.1>]
    
(This runs `rabbitmqctl list_queues ...` against the first node).

The main node (`pid`) is rmq2, and the replica is on rmq1 (and is fully synchronized).

What happens when we bounce all three nodes, serially?

    $ ./lab bounce 3 2 1
    

When rmq2 (the "owning" node) goes down, we see this on the producer:

    2020/08/20 13:31:21 --[ tx.COMPLETE 9 100% ]----------
    2020/08/20 13:31:21 --[ tx.START    10   0% ]----------
    2020/08/20 13:31:29 --[ tx.COMPLETE 10 100% ]----------
    2020/08/20 13:31:29 --[ tx.START    11   0% ]----------

    2020/08/20 13:31:32 last sent '11|351'
    2020/08/20 13:31:32 connecting to amqp://rmq:sekrit@localhost:25672/...
    2020/08/20 13:31:32 opening comms channel...
    2020/08/20 13:31:32 TX: sending messages to rmq-default via rmq-default...
    2020/08/20 13:31:32 initiating TX ->rmq-default @11|351
    2020/08/20 13:31:38 --[ tx.COMPLETE 11 100% ]----------
    2020/08/20 13:31:38 --[ tx.START    12   0% ]----------

On the consumer side, we see no message loss:

    2020/08/20 13:31:29 --[ rx.COMPLETE 10 100% ]----------
    2020/08/20 13:31:29 --[ rx.START    11   0% ]----------
    2020/08/20 13:31:29 batch 0 ... 10 COMPLETE
    2020/08/20 13:31:29 batch 11 IN PROGRESS
    2020/08/20 13:31:38 --[ rx.COMPLETE 11 100% ]----------
    2020/08/20 13:31:38 --[ rx.START    12   0% ]----------
    2020/08/20 13:31:38 batch 0 ... 11 COMPLETE
    2020/08/20 13:31:38 batch 12 IN PROGRESS

Notably, the consumer _does not_ disconnect when rmq2 shuts down, but it _does_ when rmq1 gets bounced:

    2020/08/20 13:33:10 batch 0 ... 18 COMPLETE
    2020/08/20 13:33:10 batch 19 IN PROGRESS
    2020/08/20 13:33:10 connecting to amqp://rmq:sekrit@localhost:25672/...
    2020/08/20 13:33:10 opening comms channel...
    2020/08/20 13:33:10 RX: receiving messages from rmq-default via rmq-default...
    2020/08/20 13:33:12 --[ rx.COMPLETE 19 100% ]----------
    2020/08/20 13:33:12 --[ rx.START    20   0% ]----------
    2020/08/20 13:33:12 batch 0 ... 19 COMPLETE
    2020/08/20 13:33:12 batch 20 IN PROGRESS
    2020/08/20 13:33:21 --[ rx.COMPLETE 20 100% ]----------

## Testing HA - "Scale-Across" Migration

With our application still running (up around batch 30 or so), we can try a variation on the outage / failure model by scaling across to new nodes.

The way this works is we take the three new nodes (4, 5, and 6) introduce each of them to the cluster and then decommission one of the old nodes (1, 2, and 3).

It looks like this:

    a)            [1 2 3]       [4] [5] [6]
    b)            [1 2 3 4]         [5] [6]
    c) [1]          [2 3 4]         [5] [6]
    d) [1]          [2 3 4 5]           [6]
    e) [1] [2]        [3 4 5]           [6]
    f) [1] [2]        [3 4 5 6]
    g) [1] [2] [3]      [4 5 6]


(Note that we keep the `rabbit` app _shut off_ on the single-node clusters, to avoid accidentally getting traffic from the eager load balancer)

    $ ./lab roll 4 5 6

The cluster starts out in step (a).

Then it transitions to step (b):

    INTRODUCING rmq4 to cluster via rmq1...
    Resetting node rabbit@rmq4 ...
    Clustering node rabbit@rmq4 with rabbit@rmq1
    Starting node rabbit@rmq4 ...
    ==============================================

Then, we remove rmq1 from the cluster, moving on to step (c):

    STEPPING DOWN rmq1...
    Stopping rabbit application on node rabbit@rmq1 ...
    Resetting node rabbit@rmq1 ...
    ==============================================

Then it transitions to step (d):

    INTRODUCING rmq5 to cluster via rmq2...
    Resetting node rabbit@rmq5 ...
    Clustering node rabbit@rmq5 with rabbit@rmq2
    Starting node rabbit@rmq5 ...
    ==============================================

Then, we remove rmq2 from the cluster, moving on to step (e):

    STEPPING DOWN rmq2...
    Stopping rabbit application on node rabbit@rmq2 ...
    Resetting node rabbit@rmq2 ...
    ==============================================

Then it transitions to step (f):

    INTRODUCING rmq6 to cluster via rmq3...
    Resetting node rabbit@rmq6 ...
    Clustering node rabbit@rmq6 with rabbit@rmq3
    Starting node rabbit@rmq6 ...
    ==============================================

Finally, we remove rmq3 from the cluster, ending at step (f):

    STEPPING DOWN rmq3...
    Stopping rabbit application on node rabbit@rmq3 ...
    Resetting node rabbit@rmq3 ...
    ==============================================

The producer gets disconnected when the node it was connected to (through the load balancer) goes down:

    2020/08/20 13:41:39 --[ tx.START    68   0% ]----------
    2020/08/20 13:41:50 --[ tx.COMPLETE 68 100% ]----------
    2020/08/20 13:41:50 --[ tx.START    69   0% ]----------

    2020/08/20 13:41:55 last sent '69|673'
    2020/08/20 13:41:55 connecting to amqp://rmq:sekrit@localhost:25672/...
    2020/08/20 13:41:55 opening comms channel...
    2020/08/20 13:41:55 TX: sending messages to rmq-default via rmq-default...
    2020/08/20 13:41:56 initiating TX ->rmq-default @69|673
    2020/08/20 13:41:58 --[ tx.COMPLETE 69 100% ]----------

The consumer likewise gets disconnected:

    2020/08/20 13:41:39 batch 68 IN PROGRESS
    2020/08/20 13:41:50 --[ rx.COMPLETE 68 100% ]----------
    2020/08/20 13:41:50 --[ rx.START    69   0% ]----------
    2020/08/20 13:41:50 batch 0 ... 68 COMPLETE
    2020/08/20 13:41:50 batch 69 IN PROGRESS
    2020/08/20 13:41:55 =========[ the story so far... ]=============
    2020/08/20 13:41:55 batch 0 ... 68 COMPLETE
    2020/08/20 13:41:55 batch 69 IN PROGRESS
    2020/08/20 13:41:55 connecting to amqp://rmq:sekrit@localhost:25672/...
    2020/08/20 13:41:55 opening comms channel...
    2020/08/20 13:41:55 RX: receiving messages from rmq-default via rmq-default...
    2020/08/20 13:41:58 --[ rx.COMPLETE 69 100% ]----------
    2020/08/20 13:41:58 --[ rx.START    70   0% ]----------
    2020/08/20 13:41:58 batch 0 ... 69 COMPLETE
    2020/08/20 13:41:58 batch 70 IN PROGRESS

However, thanks to the durability of the queue / exchange, the mirroring of the queue, the use of persistent message delivery, explicit consumer-initiated message acknowledgment, and publisher confirms, no messages are lost.
