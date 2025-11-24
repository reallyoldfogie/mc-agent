module github.com/spbinns/mc-agent

go 1.23.3

// replace github.com/Tnze/go-mc => /root/docker/github.com/spbinns/go-mc
// replace github.com/Tnze/go-mc => /root/docker/github.com/Tnze/go-mc 

// replace github.com/spbinns/go-mc/bot/world/entity => /root/docker/github.com/spbinns/go-mc/bot/world/entity

require (
	// github.com/Tnze/go-mc v1.21.1
	github.com/Tnze/go-mc v1.20.3-0.20240907175330-9a1f5431370e
	//	github.com/Tnze/go-mc v1.16.5-pre2
	github.com/google/uuid v1.3.0
	github.com/maxsupermanhd/go-mc-ms-auth v0.0.0-20211219200017-19507a05285a
)
