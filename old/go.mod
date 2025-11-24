module github.com/spbinns/mc-agent

go 1.16

// replace github.com/Tnze/go-mc => /root/docker/github.com/twitchyliquid64/go-mc
// replace github.com/Tnze/go-mc => /root/docker/github.com/spbinns/go-mc

replace github.com/beefsack/go-astar => /root/docker/github.com/beefsack/go-astar

require (
	github.com/Tnze/go-mc v1.16.1
	github.com/beefsack/go-astar v0.0.0-20200827232313-4ecf9e304482
	github.com/google/uuid v1.1.4
	github.com/ugjka/cleverbot-go v0.0.0-20181113132206-820458cebd7b
	golang.org/dl v0.0.0-20210311202548-10a9b498f331 // indirect
)
