
# NATS Queues

---

## Two types of queue workers

1. Standard queue with DLQ
2. Long running work queue

---

## Standard queue with DLQ

- Similar to SQS style work queues
- Very simple to implement DLQ as it is just built into NATS
- Length of work must be known and short. In this example 5 seconds, 10 seconds, and 15 seconds
  - Although since each consumer configures it's own timeouts you can probably have some interesting patterns

[Example](https://excalidraw.com/#json=VW9N6JfUwNx-lxXo2fu3a,Fwh_76wuAh1eVL14c60zxQ)

---

## Long running work queue

- Instead of receiving work from a queue workers lobby for work
- That work is expected to run for a very long time but might be terminated at any time
  - if work is terminated it should be available to be reclaimed at a later date by another worker

---

## Long running work queue implementation

- Instead of using a queue, use a key value store
  - Redis has something available for this called a [redlock](https://redis.io/docs/latest/develop/use/patterns/distributed-locks/)/[redsync](https://github.com/go-redsync/redsync)
  - But we can simply implement it in NATS as well

- In NATS we use a separate KV bucket with a TTL
- When a worker wants to claim some work it attempts to write a lock to the KV bucket
- The worker updates the lock every X seconds to keep the lock owned
- If the worker fails to update the lock it will be deleted by the TTL and available to reclaim

[Example](https://excalidraw.com/#json=4ic8iBRfA7jTXOJ_8mES1,WhNaY7aejWlxNO_lMFslzQ)
