package main

import (
	"flag"
	"log"

	"github.com/redhat-et/docsclaw/internal/skillpuller"
)

func main() {
	port := flag.Int("port", 9100, "HTTP listen port")
	skillsDir := flag.String("skills-dir", "/data/skills", "directory to write pulled skills into")
	flag.Parse()

	srv := skillpuller.NewServer(*skillsDir, *port)
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
