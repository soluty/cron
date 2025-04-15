Fork of https://godoc.org/github.com/robfig/cron  

Changes:  
Jobs has a context  
Jobs recover panic by default  
Jobs can have a description (label)  
Can have a job that only run once using cron.Once  
Can change the *time.Location while running  
Do not sort all entries at evey iteration  
Tests runs in under a second (instead of over a minute)  

```go
go test -v ./...
go clean -testcache && go test -race ./...
```