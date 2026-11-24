package protocol

// SDRemoteCall invokes Lua code with a packed argument array.
type SDRemoteCall struct {
	Code string
	Args LuaVar
}

func (v SDRemoteCall) MarshalPortable(writer *PortableWriter) error {
	writer.String(v.Code)
	return v.Args.MarshalPortable(writer)
}

func (v *SDRemoteCall) UnmarshalPortable(reader *PortableReader) error {
	v.Code = reader.String()
	return v.Args.UnmarshalPortable(reader)
}

// SDRemoteCallResult carries multiline errors and multiple Lua return values.
type SDRemoteCallResult struct {
	Error   []string
	VarList []LuaVar
}

func (v SDRemoteCallResult) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.Error)))
	for _, item := range v.Error {
		writer.String(item)
	}
	writer.SizeTag(uint64(len(v.VarList)))
	for _, item := range v.VarList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDRemoteCallResult) UnmarshalPortable(reader *PortableReader) error {
	errorCount := readPortableCount(reader, 1<<20, "remote error")
	if reader.Err() != nil {
		return reader.Err()
	}
	if errorCount == 0 {
		v.Error = nil
	} else {
		v.Error = make([]string, errorCount)
	}
	for i := range v.Error {
		v.Error[i] = reader.String()
	}
	valueCount := readPortableCount(reader, 1<<20, "remote result")
	if reader.Err() != nil {
		return reader.Err()
	}
	if valueCount == 0 {
		v.VarList = nil
	} else {
		v.VarList = make([]LuaVar, valueCount)
	}
	for i := range v.VarList {
		if err := v.VarList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

// SDSendNotify is a queued Lua notification.
type SDSendNotify struct {
	Key         uint64
	SeqID       uint64
	VarList     LuaVar
	WaitConsume bool
}

func (v SDSendNotify) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Key)
	writer.Uint64(v.SeqID)
	if err := v.VarList.MarshalPortable(writer); err != nil {
		return err
	}
	writer.Bool(v.WaitConsume)
	return writer.Err()
}

func (v *SDSendNotify) UnmarshalPortable(reader *PortableReader) error {
	v.Key = reader.Uint64()
	v.SeqID = reader.Uint64()
	if err := v.VarList.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.WaitConsume = reader.Bool()
	return reader.Err()
}
