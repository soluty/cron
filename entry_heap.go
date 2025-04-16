package cron

type EntryHeap []*Entry

func (h *EntryHeap) Less(i, j int) bool {
	e1, e2 := (*h)[i], (*h)[j]
	if e1.Next.IsZero() || !e1.Active {
		return false
	} else if e2.Next.IsZero() || !e2.Active {
		return true
	}
	return e1.Next.Before(e2.Next)
}

func (h *EntryHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func (h *EntryHeap) Len() int {
	return len(*h)
}

func (h *EntryHeap) Pop() (v any) {
	*h, v = (*h)[:h.Len()-1], (*h)[h.Len()-1]
	return
}

func (h *EntryHeap) Push(v any) {
	*h = append(*h, v.(*Entry))
}

func (h *EntryHeap) Peek() *Entry {
	if len(*h) == 0 {
		return nil
	}
	return (*h)[0]
}
