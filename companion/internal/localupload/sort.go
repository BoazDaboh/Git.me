package localupload

import "sort"

func sortFiles(s *Snapshot) {
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	sort.Slice(s.Excluded, func(i, j int) bool { return s.Excluded[i].Path < s.Excluded[j].Path })
}
