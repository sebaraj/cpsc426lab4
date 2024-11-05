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
```
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
```

Command(500 milliseconds TTL): `go run cmd/stress/tester.go --shardmap shardmaps/test-5-node.json --get-qps 10 --set-qps 10 --ttl 500ms`
```
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
```

Conclusion:

After running the two experiments, the results "show" (this is no where enough data to conclude, by
eye-balling) that the TTL strategy is working as expected, where the longer TTLs reduce the observed
latency, as the server is not "wasting" time deleting expired entries.

#### Experiment #2

In this experiment, we'll test how the system performs when shards are dynamically migrated between 
nodes during operation aka Part C. This will simulate real-world scenarios where load balancing or 
node failures might require shard redistribution, showing load balancing statefulness.

Initial ShardMap: `shardmaps/initial-4-node.json` <br>
Migrated ShardMap: `shardmaps/migrated-4-node.json`

Results:
Command: `scripts/run-cluster.sh shardmaps/initial-4-node.json`
```
INFO[2024-11-05T01:24:38-05:00] Running Set stress test at 10 QPS
INFO[2024-11-05T01:24:38-05:00] Running Get stress test at 10 QPS
INFO[2024-11-05T01:24:38-05:00] [sampled] get OK                              key=ottsbnhjsh latency_us=17042
INFO[2024-11-05T01:24:39-05:00] [sampled] get OK                              key=fqltwpoyyk latency_us=676
INFO[2024-11-05T01:24:41-05:00] [sampled] set OK                              key=xzdhkaubad latency_us=509
INFO[2024-11-05T01:24:42-05:00] [sampled] get OK                              key=hxcznrrbmv latency_us=1542
INFO[2024-11-05T01:24:43-05:00] [sampled] get OK                              key=qipzfuadiy latency_us=683
INFO[2024-11-05T01:24:44-05:00] [sampled] get OK                              key=ajmtiyckbn latency_us=493
INFO[2024-11-05T01:24:45-05:00] [sampled] get OK                              key=tnrbfczvuh latency_us=1175
INFO[2024-11-05T01:24:46-05:00] [sampled] set OK                              key=iiahcojlgd latency_us=2263
INFO[2024-11-05T01:24:47-05:00] [sampled] get OK                              key=tybspcyjig latency_us=762
INFO[2024-11-05T01:24:48-05:00] [sampled] get OK                              key=gpbyvqehjn latency_us=741
INFO[2024-11-05T01:24:49-05:00] [sampled] get OK                              key=zgkknakqav latency_us=933
INFO[2024-11-05T01:24:50-05:00] [sampled] get OK                              key=njogblkecm latency_us=889
INFO[2024-11-05T01:24:51-05:00] [sampled] get OK                              key=iuituwnjqh latency_us=784
INFO[2024-11-05T01:24:52-05:00] [sampled] set OK                              key=tgffsxpydt latency_us=1080
INFO[2024-11-05T01:24:53-05:00] [sampled] get OK                              key=cbvhhthgtp latency_us=736
INFO[2024-11-05T01:24:54-05:00] [sampled] get OK                              key=msdphvwhds latency_us=1131
INFO[2024-11-05T01:24:55-05:00] [sampled] get OK                              key=xydecumifk latency_us=660
INFO[2024-11-05T01:24:56-05:00] [sampled] get OK                              key=hlyjwfilxc latency_us=475
INFO[2024-11-05T01:24:57-05:00] [sampled] get OK                              key=dxrtnyzrie latency_us=400
INFO[2024-11-05T01:24:58-05:00] [sampled] get OK                              key=scgucgvnli latency_us=546
INFO[2024-11-05T01:24:59-05:00] [sampled] get OK                              key=pufelldjho latency_us=659
INFO[2024-11-05T01:25:00-05:00] [sampled] get OK                              key=mnhhvtwqcs latency_us=786
INFO[2024-11-05T01:25:01-05:00] [sampled] set OK                              key=rzfandufvy latency_us=475
INFO[2024-11-05T01:25:02-05:00] [sampled] get OK                              key=iwygvebghr latency_us=502
INFO[2024-11-05T01:25:04-05:00] [sampled] set OK                              key=svrsozppuw latency_us=728
INFO[2024-11-05T01:25:05-05:00] [sampled] set OK                              key=lzptcsebrm latency_us=546
INFO[2024-11-05T01:25:06-05:00] [sampled] get OK                              key=ivtygqdqsk latency_us=798
INFO[2024-11-05T01:25:07-05:00] [sampled] get OK                              key=apvpbszyks latency_us=387
INFO[2024-11-05T01:25:08-05:00] [sampled] set OK                              key=vxhuuqikoj latency_us=2704
```

Dynamically updated to: `migrated-4-node.json`
```
INFO[2024-11-05T01:25:09-05:00] [sampled] get OK                              key=znxkrlydta latency_us=2612
INFO[2024-11-05T01:25:10-05:00] [sampled] get OK                              key=iwghlgozlc latency_us=539
INFO[2024-11-05T01:25:11-05:00] [sampled] get OK                              key=kxkmnmzgfa latency_us=483
INFO[2024-11-05T01:25:12-05:00] [sampled] get OK                              key=zxvrcqcmtr latency_us=746
INFO[2024-11-05T01:25:13-05:00] [sampled] get OK                              key=bqtxmbbwzk latency_us=950
INFO[2024-11-05T01:25:14-05:00] [sampled] get OK                              key=uhtyzgwodl latency_us=584
INFO[2024-11-05T01:25:15-05:00] [sampled] get OK                              key=wljvwlpnov latency_us=709
INFO[2024-11-05T01:25:16-05:00] [sampled] get OK                              key=gejizltuon latency_us=813
INFO[2024-11-05T01:25:17-05:00] [sampled] get OK                              key=gahtmrvuvs latency_us=523
INFO[2024-11-05T01:25:18-05:00] [sampled] set OK                              key=sjpoeaccgx latency_us=683
INFO[2024-11-05T01:25:20-05:00] [sampled] get OK                              key=lqvclvfiql latency_us=569
INFO[2024-11-05T01:25:21-05:00] [sampled] set OK                              key=rqtrhqymjx latency_us=868
INFO[2024-11-05T01:25:22-05:00] [sampled] get OK                              key=vaymsfcghu latency_us=816
INFO[2024-11-05T01:25:23-05:00] [sampled] get OK                              key=vqtntuufse latency_us=699
INFO[2024-11-05T01:25:24-05:00] [sampled] set OK                              key=wvfxqidoki latency_us=874
INFO[2024-11-05T01:25:25-05:00] [sampled] set OK                              key=tqnirpdxsr latency_us=764
INFO[2024-11-05T01:25:26-05:00] [sampled] get OK                              key=uqbxwjtxof latency_us=534
INFO[2024-11-05T01:25:27-05:00] [sampled] get OK                              key=turmkcynkg latency_us=479
INFO[2024-11-05T01:25:28-05:00] [sampled] get OK                              key=masibjrbmg latency_us=473
INFO[2024-11-05T01:25:29-05:00] [sampled] set OK                              key=msnkpaxaao latency_us=547
INFO[2024-11-05T01:25:30-05:00] [sampled] get OK                              key=zvrapvewbk latency_us=460
INFO[2024-11-05T01:25:31-05:00] [sampled] get OK                              key=jliujtrxdj latency_us=578
INFO[2024-11-05T01:25:32-05:00] [sampled] set OK                              key=szccoktftw latency_us=543
INFO[2024-11-05T01:25:33-05:00] [sampled] get OK                              key=ymiwlvqbpq latency_us=577
INFO[2024-11-05T01:25:34-05:00] [sampled] get OK                              key=gfwvzsshgl latency_us=479
INFO[2024-11-05T01:25:35-05:00] [sampled] get OK                              key=pvruholfst latency_us=491
INFO[2024-11-05T01:25:36-05:00] [sampled] get OK                              key=fqaeqbxsjb latency_us=826
INFO[2024-11-05T01:25:37-05:00] [sampled] set OK                              key=hisxzonyya latency_us=490
INFO[2024-11-05T01:25:38-05:00] [sampled] set OK                              key=zlanjwhfbo latency_us=790
```

Conclusion:

The consistency over resuts shows that the application maintains its state correctly during the shard migrations.

#### Experiment 3

In this test I aim to evaluate the efficiency of read locking in our distributed key-value store, particularly 
its ability to handle concurrent read operations while write operations are in progress.

Command1: `go run cmd/stress/tester.go --shardmap shardmaps/test-5-node.json --get-qps 200 --set-qps 10 --ttl 30s --duration 30s`
```
INFO[2024-11-05T01:43:40-05:00] Running Set stress test at 10 QPS
INFO[2024-11-05T01:43:40-05:00] Running Get stress test at 200 QPS
INFO[2024-11-05T01:43:40-05:00] [sampled] get OK                              key=cvrqmwqgsc latency_us=3561
INFO[2024-11-05T01:43:41-05:00] [sampled] get OK                              key=cjiqsdhzqf latency_us=480
INFO[2024-11-05T01:43:42-05:00] [sampled] get OK                              key=zchvkodzms latency_us=505
INFO[2024-11-05T01:43:43-05:00] [sampled] get OK                              key=xophksdlts latency_us=355
INFO[2024-11-05T01:43:44-05:00] [sampled] get OK                              key=gnxbzaugfv latency_us=1089
INFO[2024-11-05T01:43:45-05:00] [sampled] get OK                              key=mhsekqjicp latency_us=563
INFO[2024-11-05T01:43:46-05:00] [sampled] get OK                              key=ihtceqtzgj latency_us=532
INFO[2024-11-05T01:43:47-05:00] [sampled] get OK                              key=pbupshtjvq latency_us=508
INFO[2024-11-05T01:43:48-05:00] [sampled] get OK                              key=yyjgzbmwaq latency_us=659
INFO[2024-11-05T01:43:49-05:00] [sampled] get OK                              key=frdizvbxok latency_us=416
INFO[2024-11-05T01:43:50-05:00] [sampled] get OK                              key=lgetrkmpmv latency_us=436
INFO[2024-11-05T01:43:51-05:00] [sampled] get OK                              key=xxfheugpno latency_us=512
INFO[2024-11-05T01:43:52-05:00] [sampled] get OK                              key=ecqhpzppyl latency_us=2302
INFO[2024-11-05T01:43:53-05:00] [sampled] get OK                              key=ytxbwpzffh latency_us=349
INFO[2024-11-05T01:43:54-05:00] [sampled] get OK                              key=odidkhfdzx latency_us=682
INFO[2024-11-05T01:43:55-05:00] [sampled] get OK                              key=cczdcuatpa latency_us=570
INFO[2024-11-05T01:43:56-05:00] [sampled] get OK                              key=jcvexwbeup latency_us=545
INFO[2024-11-05T01:43:57-05:00] [sampled] get OK                              key=pvuhjtosvk latency_us=840
INFO[2024-11-05T01:43:58-05:00] [sampled] get OK                              key=ayhznqbijj latency_us=1039
INFO[2024-11-05T01:43:59-05:00] [sampled] get OK                              key=nqltzrnyjh latency_us=476
INFO[2024-11-05T01:44:00-05:00] [sampled] get OK                              key=oziufcpsob latency_us=466
INFO[2024-11-05T01:44:01-05:00] [sampled] get OK                              key=xctkdmbmox latency_us=788
INFO[2024-11-05T01:44:02-05:00] [sampled] get OK                              key=thlajenpov latency_us=837
INFO[2024-11-05T01:44:03-05:00] [sampled] get OK                              key=yljxrtnwwo latency_us=367
INFO[2024-11-05T01:44:04-05:00] [sampled] get OK                              key=urggnzgwoc latency_us=810
INFO[2024-11-05T01:44:05-05:00] [sampled] get OK                              key=vcstaijlub latency_us=469
INFO[2024-11-05T01:44:06-05:00] [sampled] get OK                              key=oklkkjkxjq latency_us=689
INFO[2024-11-05T01:44:07-05:00] [sampled] get OK                              key=qxcnsbqjao latency_us=446
INFO[2024-11-05T01:44:08-05:00] [sampled] get OK                              key=hgfptgsfta latency_us=586
INFO[2024-11-05T01:44:09-05:00] [sampled] get OK                              key=xlvzjbcayl latency_us=342
```

Command2: `go run cmd/stress/tester.go --shardmap shardmaps/test-5-node.json --get-qps 10 --set-qps 50 --ttl 30s --duration 30s`
```
INFO[2024-11-05T01:43:40-05:00] Running Set stress test at 50 QPS
INFO[2024-11-05T01:43:40-05:00] Running Get stress test at 10 QPS
INFO[2024-11-05T01:43:40-05:00] [sampled] get OK                              key=zaowsxapnb latency_us=12044
INFO[2024-11-05T01:43:41-05:00] [sampled] set OK                              key=dautudykio latency_us=484
INFO[2024-11-05T01:43:42-05:00] [sampled] set OK                              key=tgsbhlbmpe latency_us=241
INFO[2024-11-05T01:43:43-05:00] [sampled] set OK                              key=kysnurwqki latency_us=703
INFO[2024-11-05T01:43:44-05:00] [sampled] set OK                              key=dlxjcshcjo latency_us=498
INFO[2024-11-05T01:43:45-05:00] [sampled] set OK                              key=yotqjfvoxv latency_us=737
INFO[2024-11-05T01:43:46-05:00] [sampled] set OK                              key=biafwcxshf latency_us=860
INFO[2024-11-05T01:43:47-05:00] [sampled] set OK                              key=igfnyewcwu latency_us=1247
INFO[2024-11-05T01:43:48-05:00] [sampled] get OK                              key=sjqoyqzgms latency_us=689
INFO[2024-11-05T01:43:49-05:00] [sampled] get OK                              key=dbyetybons latency_us=851
INFO[2024-11-05T01:43:50-05:00] [sampled] get OK                              key=ctvjcxbyii latency_us=952
INFO[2024-11-05T01:43:51-05:00] [sampled] set OK                              key=ohkhxdphws latency_us=1015
INFO[2024-11-05T01:43:52-05:00] [sampled] set OK                              key=xhaerpqerd latency_us=572
INFO[2024-11-05T01:43:53-05:00] [sampled] set OK                              key=dkkdpwoajz latency_us=610
INFO[2024-11-05T01:43:54-05:00] [sampled] set OK                              key=wbfkuhawho latency_us=958
INFO[2024-11-05T01:43:55-05:00] [sampled] set OK                              key=ylpsrfqowu latency_us=583
INFO[2024-11-05T01:43:56-05:00] [sampled] set OK                              key=ikfooknooz latency_us=942
INFO[2024-11-05T01:43:57-05:00] [sampled] set OK                              key=gfonyztsqa latency_us=2186
INFO[2024-11-05T01:43:58-05:00] [sampled] get OK                              key=jmrhzpdhpn latency_us=586
INFO[2024-11-05T01:43:59-05:00] [sampled] get OK                              key=smsxgaavnb latency_us=1667
INFO[2024-11-05T01:44:00-05:00] [sampled] set OK                              key=grniueilca latency_us=1141
INFO[2024-11-05T01:44:01-05:00] [sampled] set OK                              key=xntyvimobn latency_us=981
INFO[2024-11-05T01:44:02-05:00] [sampled] set OK                              key=ohajvpualm latency_us=690
INFO[2024-11-05T01:44:03-05:00] [sampled] set OK                              key=xasbolwheg latency_us=754
INFO[2024-11-05T01:44:04-05:00] [sampled] set OK                              key=yzejvoxyeb latency_us=1577
INFO[2024-11-05T01:44:05-05:00] [sampled] set OK                              key=epibllfvjp latency_us=841
INFO[2024-11-05T01:44:06-05:00] [sampled] set OK                              key=igwavouqsc latency_us=731
INFO[2024-11-05T01:44:07-05:00] [sampled] set OK                              key=azjyfvyxlm latency_us=481
INFO[2024-11-05T01:44:08-05:00] [sampled] set OK                              key=knxioagyoq latency_us=820
INFO[2024-11-05T01:44:09-05:00] [sampled] set OK                              key=elrnrpteln latency_us=1251
```

Conclusion:

**Read-heavy workload (200 GET QPS, 10 SET QPS):** <br>
    GET latencies: 342μs to 3561μs <br>

**Write-heavy workload (10 GET QPS, 50 SET QPS):** <br>
    SET latencies: 241μs to 2186μs <br>
    GET latencies: 586μs to 12044μs <br>

In the read-heavy workload, we observe low average GET latency (~768μs) despite high read concurrency (200 QPS).
On the other hand, the write-heavy workload shows that write operations (SET) don't significantly impact read 
performance. While GET latencies are higher in this scenario, they remain reasonable given the write-intensive 
environment. Note, the system maintains high throughput and low latency even under increased load.
