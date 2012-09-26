package main

import (
	"io"
	"encoding/gob"
)

func BinIndex(w io.Writer, index *Index) {
	encoder := gob.NewEncoder(w)
	encoder.Encode(index)
}

func RepIndex(index *Index) string {
	s := "Index:\n"

	for i := range index.Sites {
		s += index.Sites[i].URL + "\n"
		for j := range index.Sites[i].Pages {
			s += "\t" + index.Sites[i].Pages[j].Path + "\n"
			for k := range index.Sites[i].Pages[j].Links {
				s += "\t\t" + index.Sites[i].Pages[j].Links[k] + "\n"
			}
		}
	}

	return s + "\n\n"
}
