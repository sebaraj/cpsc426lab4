# Lab 4 - Discussion

## A4

#### What tradeoffs did you make with your TTL strategy? How fast does it clean up data and how expensive is it in terms of number of keys?

When considering a TTL strategy, there were a few factors to consider. First, how much should we
prioritize additional memory usage to store the TTL information? Second, how much should we
prioritize the time it takes to clean up the data? There was a trivial solution to just interate
through all KV pairs and check their TTL, with a $O(n)$ time complexity and minimal memory overhead,
as we are storing the KV entries as keys with pointer values to their raw value entries. There is
also no overhead in terms of additional time or space when setting new KV pairs or deleting existing
pairs. However, unless the expected behavior of the entries is to quickly expire, iterating through
all entries is very expensive. As such, we decided to utilize a min-heap to sort entries by their
TTL. This allows us to quickly find the next few entries which need to be deleted, at the cost of
$O(n)$ space complexity to store the heap. The access time to delete $k$ entries is $O(k \log n)$,
and when $0 < k < n$, which is faster than the $O(n)$ time complexity when only a fraction of entries are
expired at each ttl check interval. In addition, the set/delete operations now take $O(\log n)$ time
(container/heap documentation). Given that we expect such behavior, this is a reasonable tradeoff to
prevent the unneccsarily search through all entries when checking ttl expiration.

#### If the server was write intensive, would you design it differently? How so?

If the server was more write intensive, the additional overhead of the min-heap would be more
costly, and as such, I would consider reverting to the trivial solution of iterating through all
elements (check: $O(n)$, set/delete: $O(1)$), as the min-heap would be most costly with its
$O(log(n))$ set. Given this approach, I would simply reduce the frequency of TTL checks so that all
entries were not iterated over as often (remember, get will invalidate expired entries, regardless
of their presence).

## B2

#### What flaws could you see in the load balancing strategy you chose? Consider cases where nodes may naturally have load imbalance already -- a node may have more CPUs available, or a node may have more shards assigned.

When using a round robin strategy from the client side, the client may not be aware of additional
load on the server by additional clients. Consider a scenario with 10 clients, all of which try to
connect to Node 1 to access Shard A. While all clients may evenly distribute their subsequent
requests to other nodes with shard A, they all fail to evenly distribute their initial requests with
respect to each other. In addition, trivial, static round robin strategies do not consider
differences in the nodes/servers' capabilities, such as CPU, memory, or network bandwidth. As such,
an even distribution across all nodes may overload some nodes while underutilizing others.

#### What other strategies could help in this scenario? You may assume you can make any changes to the protocol or cluster operation.

In order to help with this scenario, I would implement dynamic load balancing. Utitilizing either a
calculating node load factor or average respond time, which the nodes would report to the client,
allowing the client to make more informed decisions on which node to connect to. This would allow
the client to connect to the node with the least load, and prevent overloading of any one node.

## B3

#### For this lab you will try every node until one succeeds. What flaws do you see in this strategy? What could you do to help in these scenarios?

When trying every node until one succeeds, the client may lead to cascading failure. Consider the
case where numerous clients simultaneously overload node 1, causing it to fail. Then, all clients
will attempt to connect to node 2, which will also subsequently fail due to the overwhelming load,
and this load will cascade all nodes which possess the desired key/shard. To prevent this, we could
implement a backoff strategy, where the client will wait a defined amount of time before attempting
the next node, with that time increasing exponentially with each failed attempt. This will allow the
nodes time to recover from the load, and prevent the cascading failure.

## B4

#### What are the ramifications of partial failures on Set calls? What anomalies could a client observe?

Partial failures on Set calls could lead to inconsistencies in the data store. Consider the scenario
where shard A exists on both Nodes 1 and 2. If a client is able to successfully set a key on shard A
in Node 1 but fails to set that same key on shard A in Node 2, then the client will observe that two
get calls to the same key will return different values. By our round robin strategy, the client will
make the first get request to shard A in Node 1, returning the recently set value; however, the
client will then make the second get request to shard A in Node 2, which will fail to return the
recently set value as the previously mentioned set call failed for this node. As such, this data
store is not consistent.

## D2

#### Experiment #1

I intend to test the performance of the TTL strategy by running the command twice, once by setting
the TTLs to rapidly expire (i.e. such that all entries are expired by the time the next TTL is
checked, taking $O(n log(n))$ time), and once by setting the TTLs to expire at a much later time in
order to better simulate the expected behavior of the data store and min-heap.

ShardMap: `shardmaps/test-5-node.json`

Server Command: `scripts/run-cluster.sh shardmaps/test-5-node.json`

Results:
Command (1 second TTL): `go run cmd/stress/tester.go --shardmap shardmaps/test-5-node.json --get-qps 10 --set-qps 10 --ttl 1s`
INFO[2024-11-04T20:02:08-05:00] Running Set stress test at 10 QPS
INFO[2024-11-04T20:02:08-05:00] Running Get stress test at 10 QPS
INFO[2024-11-04T20:02:08-05:00] [sampled] get OK key=ihsevyhicx latency_us=5838
INFO[2024-11-04T20:02:10-05:00] [sampled] get OK key=ybyybuotcl latency_us=783
INFO[2024-11-04T20:02:11-05:00] [sampled] get OK key=egqropssue latency_us=3273
INFO[2024-11-04T20:02:12-05:00] [sampled] get OK key=qqldanzcnz latency_us=1357
INFO[2024-11-04T20:02:13-05:00] [sampled] get OK key=uswctwsdyl latency_us=1703
INFO[2024-11-04T20:02:14-05:00] [sampled] set OK key=opfqhztgvh latency_us=2487
INFO[2024-11-04T20:02:15-05:00] [sampled] get OK key=egqropssue latency_us=2218
INFO[2024-11-04T20:02:16-05:00] [sampled] set OK key=qinhcssnlh latency_us=2777
INFO[2024-11-04T20:02:17-05:00] [sampled] get OK key=mppxiwnpml latency_us=4865
INFO[2024-11-04T20:02:18-05:00] [sampled] get OK key=umutfuzisk latency_us=2916
INFO[2024-11-04T20:02:19-05:00] [sampled] set OK key=tmrrnvjnma latency_us=3112
INFO[2024-11-04T20:02:20-05:00] [sampled] get OK key=ncnweikcrb latency_us=1383
INFO[2024-11-04T20:02:21-05:00] [sampled] get OK key=npvgcdrgdz latency_us=2874
INFO[2024-11-04T20:02:22-05:00] [sampled] get OK key=punqhzdwtn latency_us=1592
INFO[2024-11-04T20:02:23-05:00] [sampled] get OK key=jinkkxuxtq latency_us=1401
INFO[2024-11-04T20:02:24-05:00] [sampled] set OK key=fianuqvwcj latency_us=3433
INFO[2024-11-04T20:02:25-05:00] [sampled] get OK key=enbaoftlua latency_us=2995

Command(500 milliseconds TTL): `go run cmd/stress/tester.go --shardmap shardmaps/test-5-node.json --get-qps 10 --set-qps 10 --ttl 500ms`
INFO[2024-11-04T20:01:04-05:00] Running Get stress test at 10 QPS
INFO[2024-11-04T20:01:04-05:00] Running Set stress test at 10 QPS
INFO[2024-11-04T20:01:04-05:00] [sampled] get OK key=sfdtzyvoqo latency_us=6858
INFO[2024-11-04T20:01:05-05:00] [sampled] get OK key=jdwthwirwv latency_us=3489
INFO[2024-11-04T20:01:06-05:00] [sampled] set OK key=afiaersqnv latency_us=6618
INFO[2024-11-04T20:01:07-05:00] [sampled] get OK key=ydxivwqcnf latency_us=1513
INFO[2024-11-04T20:01:08-05:00] [sampled] set OK key=koqkcpqeze latency_us=1823
INFO[2024-11-04T20:01:09-05:00] [sampled] set OK key=muwuqaoxub latency_us=2223
INFO[2024-11-04T20:01:10-05:00] [sampled] set OK key=vnayjmeuza latency_us=4770
INFO[2024-11-04T20:01:12-05:00] [sampled] get OK key=jazyjgpllc latency_us=3418
INFO[2024-11-04T20:01:13-05:00] [sampled] set OK key=yjnlqrblmz latency_us=3926
INFO[2024-11-04T20:01:14-05:00] [sampled] get OK key=nsjocdlnhv latency_us=1450
INFO[2024-11-04T20:01:15-05:00] [sampled] get OK key=wpwrugdfsz latency_us=1863
INFO[2024-11-04T20:01:16-05:00] [sampled] get OK key=qmfyxkqxlq latency_us=2589
INFO[2024-11-04T20:01:17-05:00] [sampled] get OK key=pfjlpnemep latency_us=1971
INFO[2024-11-04T20:01:18-05:00] [sampled] get OK key=zancgtkfdy latency_us=2659
INFO[2024-11-04T20:01:19-05:00] [sampled] get OK key=vtrpkutlxj latency_us=2146
INFO[2024-11-04T20:01:20-05:00] [sampled] set OK key=ojnkkljsrn latency_us=3372
INFO[2024-11-04T20:01:21-05:00] [sampled] get OK key=tpauvxnhob latency_us=1859
INFO[2024-11-04T20:01:22-05:00] [sampled] set OK key=btbdxjvfaw latency_us=4112
INFO[2024-11-04T20:01:23-05:00] [sampled] get OK key=ncnlmeiarz latency_us=1761

Conclusion:

After running the two experiments, the results "show" (this is no where enough data to conclude, by
eye-balling) that the TTL strategy is working as expected, where the longer TTLs reduce the observed
latency, as the server is not "wasting" time deleting expired entries.

#### Experiment #2

Command: ``