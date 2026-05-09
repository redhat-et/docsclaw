package cmd

func splitDocuments(docs []string, agents int) [][]string {
	if len(docs) == 0 {
		return nil
	}
	if agents > len(docs) {
		agents = len(docs)
	}
	batchSize := len(docs) / agents
	remainder := len(docs) % agents

	var batches [][]string
	start := 0
	for i := range agents {
		size := batchSize
		if i < remainder {
			size++
		}
		batches = append(batches, docs[start:start+size])
		start += size
	}
	return batches
}
