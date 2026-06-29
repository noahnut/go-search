package analysis

type SynonymMap map[string][]string // "car" → ["automobile", "vehicle"]

func NewSynonymMap(pairs map[string][]string) SynonymMap {
	if pairs == nil {
		return SynonymMap{}
	}

	return SynonymMap(pairs)
}

func (sm SynonymMap) Get(term string) []string {
	if synonyms, ok := sm[term]; ok {
		return synonyms
	}
	return nil
}
