module github.com/reallyoldfogie/daze

go 1.24.5

//replace github.com/Tnze/go-mc => /root/docker/github.com/spbinns/go-mc

// replace github.com/spbinns/go-mc => /root/docker/github.com/spbinns/go-mc

replace github.com/reallyoldfogie/go-mc-bot => /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-bot-go

// replace github.com/Tnze/go-mc => /root/docker/github.com/Tnze/go-mc

require (
	github.com/Tnze/go-mc v1.20.3-0.20240907175330-9a1f5431370e
	github.com/maxsupermanhd/go-mc-ms-auth v0.0.0-20230820124717-22f4d907eac4
	github.com/reallyoldfogie/go-mc-bot v0.0.0-00010101000000-000000000000
)

require gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect

require (
	github.com/google/uuid v1.6.0
	github.com/iancoleman/strcase v0.2.0 // indirect
)
