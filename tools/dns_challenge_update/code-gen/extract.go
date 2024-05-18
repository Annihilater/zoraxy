package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

/*
	Usage

	git clone {repo_link_for_lego}
	go run extract.go
	//go run extract.go -- "win7"

*/

var legoProvidersSourceFolder string = "./lego/providers/dns/"
var outputDir string = "./acmedns"
var defTemplate string = `package acmedns
/*
	THIS MODULE IS GENERATED AUTOMATICALLY
	DO NOT EDIT THIS FILE
*/
import (
	"encoding/json"
	"fmt"

	"github.com/go-acme/lego/v4/challenge"
{{imports}}
)

//name is the DNS provider name, e.g. cloudflare or gandi
//JSON (js) must be in key-value string that match ConfigableFields Title in providers.json, e.g. {"Username":"far","Password":"boo"}
func GetDNSProviderByJsonConfig(name string, js string)(challenge.Provider, error){
	switch name {
{{magic}}
	default:
		return nil, fmt.Errorf("unrecognized DNS provider: %s", name)
	}
}
`

type Field struct {
	Title    string
	Datatype string
}
type ProviderInfo struct {
	Name             string   //Name of this provider
	ConfigableFields []*Field //Field that shd be expose to user for filling in
	HiddenFields     []*Field //Fields that is usable but shd be hidden from user
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	// For other errors, you may handle them differently
	return false
}

// This function define the DNS not supported by zoraxy
func getExcludedDNSProviders() []string {
	return []string{
		"edgedns",      //Too complex data structure
		"exec",         //Not a DNS provider
		"httpreq",      //Not a DNS provider
		"hurricane",    //Multi-credentials arch
		"oraclecloud",  //Evil company
		"acmedns",      //Not a DNS provider
		"selectelv2",   //Not sure why not working with our code generator
		"designate",    //OpenStack, if you are using this you shd not be using zoraxy
		"mythicbeasts", //Module require url.URL, which cannot be automatically parsed
	}
}

// Exclude list for Windows build, due to limitations for lego versions
func getExcludedDNSProvidersNT61() []string {
	return []string{
		"edgedns",      //Too complex data structure
		"exec",         //Not a DNS provider
		"httpreq",      //Not a DNS provider
		"hurricane",    //Multi-credentials arch
		"oraclecloud",  //Evil company
		"acmedns",      //Not a DNS provider
		"selectelv2",   //Not sure why not working with our code generator
		"designate",    //OpenStack, if you are using this you shd not be using zoraxy
		"mythicbeasts", //Module require url.URL, which cannot be automatically parsed

		//The following suppliers was not in lego v1.15 (Windows 7 last supported version of lego)
		"cpanel",
		"mailinabox",
		"shellrent",
	}
}

func isInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func isExcludedDNSProvider(providerName string) bool {
	return isInSlice(providerName, getExcludedDNSProviders())
}

func isExcludedDNSProviderNT61(providerName string) bool {
	return isInSlice(providerName, getExcludedDNSProvidersNT61())
}

// extractConfigStruct extracts the name of the config struct and its content as plain text from a given source code.
func extractConfigStruct(sourceCode string) (string, string) {
	// Regular expression to match the struct declaration.
	structRegex := regexp.MustCompile(`type\s+([A-Za-z0-9_]+)\s+struct\s*{([^{}]*)}`)

	// Find the first match of the struct declaration.
	match := structRegex.FindStringSubmatch(sourceCode)
	if len(match) < 3 {
		return "", "" // No match found
	}

	// Extract the struct name and its content.
	structName := match[1]
	structContent := match[2]

	return structName, structContent
}

func main() {
	// A map of provider name to information on what can be filled
	extractedProviderList := map[string]*ProviderInfo{}
	buildForWindowsSeven := (len(os.Args) > 2 && os.Args[2] == "win7")

	//Search all providers
	providers, err := filepath.Glob(filepath.Join(legoProvidersSourceFolder, "/*"))
	if err != nil {
		panic(err)
	}

	//Create output folder if not exists
	err = os.MkdirAll(outputDir, 0775)
	if err != nil {
		panic(err)
	}

	generatedConvertcode := ""
	importList := ""
	for _, provider := range providers {
		providerName := filepath.Base(provider)

		if buildForWindowsSeven {
			if isExcludedDNSProviderNT61(providerName) {
				//Ignore this provider
				continue
			}
		} else {
			if isExcludedDNSProvider(providerName) {
				//Ignore this provider
				continue
			}
		}

		//Check if {provider_name}.go exists
		providerDef := filepath.Join(provider, providerName+".go")
		if !fileExists(providerDef) {
			continue
		}

		fmt.Println("Extracting config structure for: " + providerDef)
		providerSrc, err := os.ReadFile(providerDef)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		_, strctContent := extractConfigStruct(string(providerSrc))

		//Filter and write the content to json
		/*
			Example of stctContent (Note the tab prefix)

				Token              string
				PropagationTimeout time.Duration
				PollingInterval    time.Duration
				SequenceInterval   time.Duration
				HTTPClient         *http.Client
		*/
		strctContentLines := strings.Split(strctContent, "\n")
		configKeys := []*Field{}
		hiddenKeys := []*Field{}
		for _, lineDef := range strctContentLines {
			fields := strings.Fields(lineDef)
			if len(fields) < 2 || strings.HasPrefix(fields[0], "//") {
				//Ignore this line
				continue
			}

			//Filter out the fields that is not user-filled
			switch fields[1] {
			case "*url.URL":
				fallthrough
			case "string":
				configKeys = append(configKeys, &Field{
					Title:    fields[0],
					Datatype: fields[1],
				})
			case "int":
				if fields[0] != "TTL" {
					configKeys = append(configKeys, &Field{
						Title:    fields[0],
						Datatype: fields[1],
					})
				} else if fields[0] == "TTL" {
					//haveTTLField = true
				} else {
					hiddenKeys = append(hiddenKeys, &Field{
						Title:    fields[0],
						Datatype: fields[1],
					})
				}
			case "bool":
				if fields[0] == "InsecureSkipVerify" || fields[0] == "SSLVerify" || fields[0] == "Debug" {
					configKeys = append(configKeys, &Field{
						Title:    fields[0],
						Datatype: fields[1],
					})
				} else {
					hiddenKeys = append(hiddenKeys, &Field{
						Title:    fields[0],
						Datatype: fields[1],
					})
				}
			default:
				//Not used fields
				hiddenKeys = append(hiddenKeys, &Field{
					Title:    fields[0],
					Datatype: fields[1],
				})
			}
		}
		fmt.Println(strctContent)

		extractedProviderList[providerName] = &ProviderInfo{
			Name:             providerName,
			ConfigableFields: configKeys,
			HiddenFields:     hiddenKeys,
		}

		//Generate the code for converting incoming json into target config
		codeSegment := `
	case "` + providerName + `":
		cfg := ` + providerName + `.NewDefaultConfig()
		err := json.Unmarshal([]byte(js), &cfg)
		if err != nil {
			return nil, err
		}
		return ` + providerName + `.NewDNSProviderConfig(cfg)`

		generatedConvertcode += codeSegment
		importList += `	"github.com/go-acme/lego/v4/providers/dns/` + providerName + "\"\n"
	}

	js, err := json.MarshalIndent(extractedProviderList, "", " ")
	if err != nil {
		panic(err)
	}

	fullCodeSnippet := strings.ReplaceAll(defTemplate, "{{magic}}", generatedConvertcode)
	fullCodeSnippet = strings.ReplaceAll(fullCodeSnippet, "{{imports}}", importList)

	outJsonFilename := "providers.json"
	outGoFilename := "acmedns.go"
	if buildForWindowsSeven {
		outJsonFilename = "providers_nt61.json"
		outGoFilename = "acmedns_nt61.go"
	}
	os.WriteFile(filepath.Join(outputDir, outJsonFilename), js, 0775)
	os.WriteFile(filepath.Join(outputDir, outGoFilename), []byte(fullCodeSnippet), 0775)
	fmt.Println("Output written to file")
}
