package quickfix

import (
	"bytes"
	"sort"
	"time"
)

//field stores a slice of TagValues
type field struct {
	tvs []TagValue
}

func (f *field) Tag() Tag {
	return f.tvs[0].tag
}

func (f *field) init(tag Tag, value []byte) {
	f.tvs = make([]TagValue, 1)
	f.tvs[0].init(tag, value)
}

func (f *field) write(buffer *bytes.Buffer) {
	for _, tv := range f.tvs {
		buffer.Write(tv.bytes)
	}
}

// tagOrder true if tag i should occur before tag j
type tagOrder func(i, j Tag) bool

type tagSort struct {
	tags    []Tag
	compare tagOrder
}

func (t tagSort) Len() int           { return len(t.tags) }
func (t tagSort) Swap(i, j int)      { t.tags[i], t.tags[j] = t.tags[j], t.tags[i] }
func (t tagSort) Less(i, j int) bool { return t.compare(t.tags[i], t.tags[j]) }

//FieldMap is a collection of fix fields that make up a fix message.
type FieldMap struct {
	tagLookup map[Tag]*field
	tagSort
}

// ascending tags
func normalFieldOrder(i, j Tag) bool { return i < j }

func (m *FieldMap) init() {
	m.initWithOrdering(normalFieldOrder)
}

func (m *FieldMap) initWithOrdering(ordering tagOrder) {
	m.tagLookup = make(map[Tag]*field)
	m.compare = ordering
}

//Tags returns all of the Field Tags in this FieldMap
func (m FieldMap) Tags() []Tag {
	tags := make([]Tag, 0, len(m.tagLookup))
	for t := range m.tagLookup {
		tags = append(tags, t)
	}

	return tags
}

//Get parses out a field in this FieldMap. Returned reject may indicate the field is not present, or the field value is invalid.
func (m FieldMap) Get(parser Field) MessageRejectError {
	return m.GetField(parser.Tag(), parser)
}

//Has returns true if the Tag is present in this FieldMap
func (m FieldMap) Has(tag Tag) bool {
	_, ok := m.tagLookup[tag]
	return ok
}

//GetField parses of a field with Tag tag. Returned reject may indicate the field is not present, or the field value is invalid.
func (m FieldMap) GetField(tag Tag, parser FieldValueReader) MessageRejectError {
	f, ok := m.tagLookup[tag]
	if !ok {
		return ConditionallyRequiredFieldMissing(tag)
	}

	if err := parser.Read(f.tvs[0].value); err != nil {
		return IncorrectDataFormatForValue(tag)
	}

	return nil
}

//GetBytes is a zero-copy GetField wrapper for []bytes fields
func (m FieldMap) GetBytes(tag Tag) ([]byte, MessageRejectError) {
	f, ok := m.tagLookup[tag]
	if !ok {
		return nil, ConditionallyRequiredFieldMissing(tag)
	}

	return f.tvs[0].value, nil
}

//GetBool is a GetField wrapper for bool fields
func (m FieldMap) GetBool(tag Tag) (bool, MessageRejectError) {
	var val FIXBoolean
	if err := m.GetField(tag, &val); err != nil {
		return false, err
	}
	return bool(val), nil
}

//GetInt is a GetField wrapper for int fields
func (m FieldMap) GetInt(tag Tag) (int, MessageRejectError) {
	bytes, err := m.GetBytes(tag)
	if err != nil {
		return 0, err
	}

	var val FIXInt
	if val.Read(bytes) != nil {
		err = IncorrectDataFormatForValue(tag)
	}

	return int(val), err
}

//GetTime is a GetField wrapper for utc timestamp fields
func (m FieldMap) GetTime(tag Tag) (t time.Time, err MessageRejectError) {
	bytes, err := m.GetBytes(tag)
	if err != nil {
		return
	}

	var val FIXUTCTimestamp
	if val.Read(bytes) != nil {
		err = IncorrectDataFormatForValue(tag)
	}

	return val.Time, err
}

//GetString is a GetField wrapper for string fields
func (m FieldMap) GetString(tag Tag) (string, MessageRejectError) {
	var val FIXString
	if err := m.GetField(tag, &val); err != nil {
		return "", err
	}
	return string(val), nil
}

//GetGroup is a Get function specific to Group Fields.
func (m FieldMap) GetGroup(parser FieldGroupReader) MessageRejectError {
	f, ok := m.tagLookup[parser.Tag()]
	if !ok {
		return ConditionallyRequiredFieldMissing(parser.Tag())
	}

	if _, err := parser.Read(f.tvs); err != nil {
		if msgRejErr, ok := err.(MessageRejectError); ok {
			return msgRejErr
		}
		return IncorrectDataFormatForValue(parser.Tag())
	}

	return nil
}

//SetField sets the field with Tag tag
func (m *FieldMap) SetField(tag Tag, field FieldValueWriter) *FieldMap {
	f := m.getOrCreate(tag)
	f.init(tag, field.Write())
	return m
}

//SetBool is a SetField wrapper for bool fields
func (m *FieldMap) SetBool(tag Tag, value bool) *FieldMap {
	return m.SetField(tag, FIXBoolean(value))
}

//SetInt is a SetField wrapper for int fields
func (m *FieldMap) SetInt(tag Tag, value int) *FieldMap {
	return m.SetField(tag, FIXInt(value))
}

//SetString is a SetField wrapper for string fields
func (m *FieldMap) SetString(tag Tag, value string) *FieldMap {
	return m.SetField(tag, FIXString(value))
}

//Clear purges all fields from field map
func (m *FieldMap) Clear() {
	m.tags = m.tags[0:0]
	for k := range m.tagLookup {
		delete(m.tagLookup, k)
	}
}

func (m *FieldMap) add(f *field) {
	if _, ok := m.tagLookup[f.Tag()]; !ok {
		m.tags = append(m.tags, f.Tag())
	}

	m.tagLookup[f.Tag()] = f
}

func (m *FieldMap) getOrCreate(tag Tag) *field {
	if f, ok := m.tagLookup[tag]; ok {
		return f
	}

	f := new(field)
	m.tagLookup[tag] = f
	m.tags = append(m.tags, tag)
	return f
}

//Set is a setter for fields
func (m *FieldMap) Set(field FieldWriter) *FieldMap {
	f := m.getOrCreate(field.Tag())
	f.init(field.Tag(), field.Write())
	return m
}

//SetGroup is a setter specific to group fields
func (m *FieldMap) SetGroup(field FieldGroupWriter) *FieldMap {
	f := m.getOrCreate(field.Tag())
	f.tvs = field.Write()
	return m
}

func (m *FieldMap) sortedTags() []Tag {
	sort.Sort(m)
	return m.tags
}

func (m FieldMap) write(buffer *bytes.Buffer) {
	for _, tag := range m.sortedTags() {
		if f, ok := m.tagLookup[tag]; ok {
			f.write(buffer)
		}
	}
}

func (m FieldMap) total() int {
	total := 0
	for _, fields := range m.tagLookup {
		for _, tv := range fields.tvs {
			switch tv.tag {
			case tagCheckSum: //tag does not contribute to total
			default:
				total += tv.total()
			}
		}
	}

	return total
}

func (m FieldMap) length() int {
	length := 0
	for _, fields := range m.tagLookup {
		for _, tv := range fields.tvs {
			switch tv.tag {
			case tagBeginString, tagBodyLength, tagCheckSum: //tags do not contribute to length
			default:
				length += tv.length()
			}
		}
	}

	return length
}
