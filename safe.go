package safe

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	filepath, baseDir string

	Recipients []string            `yaml:"recipients"`
	Overrides  map[string][]string `yaml:"overrides"`
	Files      []string            `yaml:"files"`
}

// LoadConfig: walk up from the current working directory, looking for a
// `safe.yml` file and build a config from it.
func LoadConfig() (Config, error) {
	for {
		if _, err := os.Stat("safe.yml"); err == nil {
			break
		}

		if cwd, err := os.Getwd(); err != nil || cwd == "/" {
			return Config{}, errors.New("no safe.yml file found")
		}

		if err := os.Chdir("../"); err != nil {
			return Config{}, err
		}

	}

	configFilepath, err := filepath.Abs("safe.yml")
	if err != nil {
		return Config{}, err
	}

	var config Config
	reader, err := os.Open(configFilepath)
	if err != nil {
		return Config{}, err
	}
	defer reader.Close()

	yamlDecoder := yaml.NewDecoder(reader)
	if err := yamlDecoder.Decode(&config); err != nil {
		return Config{}, err
	}

	config.filepath = configFilepath
	config.baseDir = filepath.Dir(configFilepath)

	if len(config.Recipients) == 0 {
		return Config{}, errors.New("Invalid config, no recipients")
	}

	return config, nil
}

// WriteConfig: write the safe config to disk
func WriteConfig(config *Config) error {
	sort.Strings(config.Files)

	configByts, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(config.filepath, configByts, 0644); err != nil {
		return err
	}

	return nil
}

// IsProtected: return whether the absolute filepath is protected
func IsProtected(checkFilepath string, config Config) (bool, error) {
	checkFilepath, err := filepath.Abs(checkFilepath)
	if err != nil {
		return false, err
	}

	relFilepath, err := filepath.Rel(config.baseDir, checkFilepath)
	if err != nil {
		return false, err
	}

	for _, protectedFilepath := range config.Files {
		if relFilepath == protectedFilepath {
			return true, nil
		}
	}

	return false, nil
}

// EnsureSuffix: ensures that the .gpg.asc suffix is present
func EnsureSuffix(filepath string) string {
	if !strings.HasSuffix(filepath, ".gpg.asc") {
		filepath += ".gpg.asc"
	}

	return filepath
}

// TrimSuffix: return the filepath with the .gpg.asc suffix removed
func TrimSuffix(filepath string) string {
	return strings.TrimSuffix(filepath, ".gpg.asc")
}

// Decrypt: decrypt a file
func Decrypt(filepath string) ([]byte, error) {
	if _, err := os.Stat(filepath); err != nil {
		return []byte(nil), err
	}

	cmd := exec.Command("gpg", "-d", filepath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return []byte(nil), err
	}

	// note: we trim the last character before returning, since it's a new
	// line added in by the command output
	return stdout.Bytes()[:stdout.Len()-1], nil
}

// DecryptToTempFile: decrypyt the src filepath into the target filepath,
// returning the decrypted content and a cleanup function.
func DecryptToFile(srcFilepath, targetFilepath string) ([]byte, func() error, error) {
	byts, err := Decrypt(srcFilepath)
	if err != nil {
		return []byte(nil), nil, err
	}

	if err := ioutil.WriteFile(targetFilepath, byts, 0644); err != nil {
		return []byte(nil), nil, err
	}

	cleanupFn := func() error {
		return os.Remove(targetFilepath)
	}

	return byts, cleanupFn, err
}

// DecryptToTempFile: decrypt to a temporary filepath
func DecryptToTempFile(srcFilepath string) (string, []byte, func() error, error) {
	tempFilepath := "/tmp/safe--" + filepath.Base(strings.Replace(srcFilepath, ".gpg.asc", "", 1))

	byts, cleanupFn, err := DecryptToFile(srcFilepath, tempFilepath)
	return tempFilepath, byts, cleanupFn, err
}

// EncryptFromFile: take the contents of an existing file and encrypt them to
// the output, deleting the original
func EncryptFromFile(srcFilepath, targetFilepath string, config Config, commit bool, action string) error {
	byts, err := ioutil.ReadFile(srcFilepath)
	if err != nil {
		return err
	}

	return Encrypt(targetFilepath, byts, config, commit, action)
}

// Commit: commit an action to the given filepaths, referencing the safe protected file
func Commit(action, filepath string, gitFilepaths []string) error {
	// NOTE: if an origin file was "protected" that had _never_ been
	// checked into source control, it will fail during the `git add`.
	// Adding a removed file that wasn't checked returns a 128 error in
	// git. To get around this, we add each file separately, and ignore
	// errors for git add
	for _, filepath := range gitFilepaths {
		exec.Command("git", "add", filepath).Run()
	}

	cmd := exec.Command("git", "commit", "-m", fmt.Sprintf("safe: %s %s", action, TrimSuffix(filepath)))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func Encrypt(filepath string, byts []byte, config Config, commit bool, action string) error {
	protected, err := IsProtected(filepath, config)
	if err != nil {
		return err
	}

	if !protected {
		config.Files = append(config.Files, filepath)
	}

	args := []string{"-a", "-e", "--yes", "--output", filepath}
	recipients, ok := config.Overrides[filepath]
	if !ok {
		recipients = config.Recipients
	}

	for _, recipient := range recipients {
		args = append(args, "-r", recipient)
	}

	cmd := exec.Command("gpg", args...)
	cmd.Stdin = bytes.NewBuffer(append(byts, '\n'))
	if err := cmd.Run(); err != nil {
		return err
	}

	if err := WriteConfig(&config); err != nil {
		return err
	}

	// if no commit is requested, return early
	if !commit {
		return nil
	}

	return Commit(action, TrimSuffix(filepath), []string{filepath, config.filepath})
}

// Edit: edit a file if it's protected, creating and protecting a file if not
func Edit(targetFilepath string, config Config, commit bool) error {
	tempFilepath, byts, cleanupFn, err := DecryptToTempFile(targetFilepath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if cleanupFn != nil {
		defer cleanupFn()
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	cmd := exec.Command(editor, tempFilepath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return err
	}

	// if the byts are the same before/after, then exit with no other changes (only if the file exists)
	editedByts, err := ioutil.ReadFile(tempFilepath)
	if err != nil {
		return err
	}

	if bytes.Equal(byts, editedByts) {
		log.Println("no changes found ...")
		return nil
	}

	return Encrypt(targetFilepath, editedByts, config, commit, "edit")
}

// Exec: execute the given command in an environment with all values decrypted from the target
func Exec(targetPath string, config Config, cmdArgs []string) error {
	if _, err := IsProtected(targetPath, config); err != nil {
		return err
	}

	if !strings.HasSuffix(TrimSuffix(targetPath), ".yml") {
		return errors.New("Only able to exec protected .yml files")
	}

	byts, err := Decrypt(targetPath)
	if err != nil {
		return err
	}

	env := make(map[string]interface{})
	if err := yaml.Unmarshal(byts, &env); err != nil {
		return err
	}

	for key, rawValue := range env {
		var value string

		switch rawValue.(type) {
		case string:
			value = rawValue.(string)
		case []string:
			value = strings.Join(rawValue.([]string), ",")
		case int:
			value = strconv.Itoa(rawValue.(int))
		default:
			value = fmt.Sprintf("%v", rawValue)
		}

		if err := os.Setenv(strings.ToUpper(key), value); err != nil {
			return err
		}
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if err != nil {
		return err
	}

	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

// Find: find all files in a directory that are protected
func Find(dir string, config Config) ([]string, error) {
	protectedFiles := make([]string, 0)

	err := filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		protected, err := IsProtected(path, config)
		if err != nil {
			return err
		}

		if !protected {
			return nil
		}

		protectedFiles = append(protectedFiles, path)
		return nil
	})
	if err != nil {
		return []string(nil), err
	}

	return protectedFiles, nil
}

// Print: prints the unencrypted file contents to stdout
func Print(targetPath string, config Config) error {
	protected, err := IsProtected(targetPath, config)
	if err != nil {
		return err
	}
	if !protected {
		return errors.New(targetPath + " is not protected")
	}

	byts, err := Decrypt(targetPath)
	if os.IsNotExist(err) {
		return errors.New(targetPath + " not found")
	}
	if err != nil {
		return err
	}

	fmt.Println(string(byts))
	return nil
}

// Protect: protect an unencrypted file
func Protect(filepath string, commit bool, config Config) error {
	protected, err := IsProtected(filepath, config)
	if err != nil {
		return err
	}

	if protected {
		return errors.New(filepath + " already protected")
	}

	origFilepath := TrimSuffix(filepath)

	// NOTE: we pass commit=false here so we can defer the commit until
	// after encryption. This allows us to commit the removal of the original file.
	if err := EncryptFromFile(origFilepath, filepath, config, false, "protect"); err != nil {
		return err
	}

	if err := os.Remove(origFilepath); err != nil {
		return err
	}

	if !commit {
		return nil
	}

	return Commit("protect", origFilepath, []string{config.filepath, origFilepath, filepath})
}

// ReencryptAll: reencrypt all files that are protected by safe
func ReencryptAll(config Config, commit bool) error {
	for _, filepath := range config.Files {
		byts, err := Decrypt(filepath)
		if err != nil {
			return err
		}

		if err := Encrypt(filepath, byts, config, commit, "reencrypt"); err != nil {
			return err
		}
	}

	return nil
}

// Remove: remove a file
func Remove(targetFilepath string, commit bool, config Config) error {
	protected, err := IsProtected(targetFilepath, config)
	if err != nil {
		return err
	}

	if !protected {
		return errors.New(targetFilepath + " is not protected")
	}

	filepaths := make([]string, 0, len(config.Files)-1)
	for _, file := range config.Files {
		if file != targetFilepath {
			filepaths = append(filepaths, file)
		}
	}
	config.Files = filepaths

	if err := os.Remove(targetFilepath); err != nil {
		return err
	}

	if err := WriteConfig(&config); err != nil {
		return err
	}

	return Commit("remove", targetFilepath, []string{targetFilepath, config.filepath})
}
