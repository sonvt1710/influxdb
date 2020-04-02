package write

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCsvData checks data that are writen in an annotated CSV file
func Test_CsvToProtocolLines(t *testing.T) {
	var tests = []struct {
		name  string
		csv   string
		lines string
		err   string
	}{
		{
			"simple1",
			"_measurement,a,b\ncpu,1,1\ncpu,b2\n",
			"cpu a=1,b=1\ncpu a=b2\n",
			"",
		},
		{
			"simple1_withSep",
			"sep=;\n_measurement;a;b\ncpu;1;1\ncpu;b2\n",
			"cpu a=1,b=1\ncpu a=b2\n",
			"",
		},
		{
			"simple2",
			"_measurement,a,b\ncpu,1,1\ncpu,\n",
			"",
			"no field data",
		},
		{
			"simple3",
			"_measurement,a,_time\ncpu,1,1\ncpu,2,invalidTime\n",
			"",
			"_time", // error in _time column
		},
		{
			"constant_annotations",
			"#constant,measurement,,cpu\n" +
				"#constant,tag,xpu,xpu1\n" +
				"#constant,tag,cpu,cpu1\n" +
				"#constant,long,of,100\n" +
				"#constant,dateTime,,2\n" +
				"x,y\n" +
				"1,2\n" +
				"3,4\n",
			"cpu,cpu=cpu1,xpu=xpu1 x=1,y=2,of=100i 2\n" +
				"cpu,cpu=cpu1,xpu=xpu1 x=3,y=4,of=100i 2\n",
			"", // no error
		},
		{
			"timezone_annotation-0100",
			"#timezone,-0100\n" +
				"#constant,measurement,cpu\n" +
				"#constant,dateTime:2006-01-02,1970-01-01\n" +
				"x,y\n" +
				"1,2\n",
			"cpu x=1,y=2 3600000000000\n",
			"", // no error
		},
		{
			"timezone_annotation_EST",
			"#timezone,EST\n" +
				"#constant,measurement,cpu\n" +
				"#constant,dateTime:2006-01-02,1970-01-01\n" +
				"x,y\n" +
				"1,2\n",
			"cpu x=1,y=2 18000000000000\n",
			"", // no error
		},
	}
	bufferSizes := []int{40, 7, 3, 1}

	for _, test := range tests {
		for _, bufferSize := range bufferSizes {
			t.Run(test.name+"_"+strconv.Itoa(bufferSize), func(t *testing.T) {
				reader := CsvToProtocolLines(strings.NewReader(test.csv))
				buffer := make([]byte, bufferSize)
				lines := make([]byte, 0, 100)
				for {
					n, err := reader.Read(buffer)
					if err != nil {
						if err == io.EOF {
							break
						}
						if test.err != "" {
							// fmt.Println(err)
							if err := err.Error(); !strings.Contains(err, test.err) {
								require.Equal(t, err, test.err)
							}
							return
						}
						require.Nil(t, err.Error())
						break
					}
					lines = append(lines, buffer[:n]...)
				}
				if test.err == "" {
					require.Equal(t, test.lines, string(lines))
				} else {
					require.Fail(t, "error message with '"+test.err+"' expected")
				}
			})
		}
	}
}

// TestCsvData_LogTableColumns checks correct logging
func TestCsvData_LogTableColumns(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	oldFlags := log.Flags()
	log.SetFlags(0)
	oldPrefix := log.Prefix()
	prefix := "::PREFIX::"
	log.SetPrefix(prefix)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	csv := "_measurement,a,b\ncpu,1,1\ncpu,b2\n"

	reader := CsvToProtocolLines(strings.NewReader(csv)).LogTableColumns(true)
	require.Equal(t, reader.skipRowOnError, false)
	require.Equal(t, reader.logTableDataColumns, true)
	// read all the data
	ioutil.ReadAll(reader)

	out := buf.String()
	// fmt.Println(out)
	messages := strings.Count(out, prefix)
	require.Equal(t, messages, 1)
}

// TestCsvData_LogTableColumns checks correct logging
func TestCsvData_LogTimeZoneWarning(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	oldFlags := log.Flags()
	log.SetFlags(0)
	oldPrefix := log.Prefix()
	prefix := "::PREFIX::"
	log.SetPrefix(prefix)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	csv := "#timezone 1\n" +
		"#constant,dateTime:2006-01-02,1970-01-01\n" +
		"_measurement,a,b\ncpu,1,1"

	reader := CsvToProtocolLines(strings.NewReader(csv))
	bytes, _ := ioutil.ReadAll(reader)

	out := buf.String()
	// fmt.Println(out)
	messages := strings.Count(out, prefix)
	require.Equal(t, messages, 1)
	require.Equal(t, string(bytes), "cpu a=1,b=1 0\n")
}

// TestCsvData_LogCsvErrors checks correct logging
func TestCsvData_SkipRowOnError(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	oldFlags := log.Flags()
	log.SetFlags(0)
	oldPrefix := log.Prefix()
	prefix := "::PREFIX::"
	log.SetPrefix(prefix)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	csv := "_measurement,a,_time\n,1,1\ncpu,2,2\ncpu,3,3a\n"

	reader := CsvToProtocolLines(strings.NewReader(csv)).SkipRowOnError(true)
	require.Equal(t, reader.skipRowOnError, true)
	require.Equal(t, reader.logTableDataColumns, false)
	// read all the data
	ioutil.ReadAll(reader)

	out := buf.String()
	// fmt.Println(out)
	messages := strings.Count(out, prefix)
	require.Equal(t, messages, 2)
}

// Test_CsvLineError checks formating of line errors
func Test_CsvLineError(t *testing.T) {
	var tests = []struct {
		err   CsvLineError
		value string
	}{
		{
			CsvLineError{Line: 1, Err: errors.New("cause")},
			"line 1: cause",
		},
		{
			CsvLineError{Line: 2, Err: CsvColumnError{"a", errors.New("cause")}},
			"line 2: column 'a': cause",
		},
	}
	for _, test := range tests {
		require.Equal(t, test.value, test.err.Error())
	}
}
