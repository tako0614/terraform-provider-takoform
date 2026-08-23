package portableconformancev3

// testEdgeFormsGroup is the family generation these tests build fixtures in.
// It is a TEST constant on purpose: the host no longer has one, because a
// compiled-in family made every cross-resource guard a silent early return for
// any other generation. A test knows which fixtures it wrote, so it may say so;
// a host must ask the candidate set it installed.
const testEdgeFormsGroup = "edge.forms.takoform.com"
