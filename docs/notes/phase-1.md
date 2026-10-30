# Phase 1 notes

[Versão em português](phase-1.pt-BR.md)

What happens when 1000 requests to reserve arrive at the same time (very common in big sales)? I wrote a test using goroutines, a WaitGroup and an atomic counter. At the end of the 1000 executions, I noticed that in some cases the counter stored the wrong quantity of sales (higher than 100, the product’s stock), i.e., I simulated “selling more than I have”. I ran it twenty times (-count=20) and it failed 17 times, one of them with 102 sales. With -race, I noticed some lines in inventory.go that read and write the stock. Basically, one goroutine read the stock and decreased it, and a second one did the same thing with an out-of-date stock. It is as if both read “current stock is 50”, when the first goroutine should read “current stock is 50” and the second “current stock is 49”. Goroutines do not have a guaranteed execution order.

I protected the store with sync.Mutex using Lock and defer Unlock. The mutex ensures that one goroutine at a time reads and changes the products map (stock).
