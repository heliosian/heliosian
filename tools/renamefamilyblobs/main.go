// Command renamefamilyblobs renames family media in the drive from parent-email names to family key hashes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path"
	"strings"

	"heliosian/internal/data"
	"heliosian/internal/directory"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const folderMime = "application/vnd.google-apps.folder"

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> is required")
	}
	source, err := data.NewSheet(map[string]string{"directory": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source, nil)
	if err != nil {
		log.Fatalf("[ERROR] load model: %v", err)
	}
	localToHash := map[string]string{}
	for _, family := range model.Families {
		for _, adult := range family.AdultEmails {
			local, _, _ := strings.Cut(adult, "@")
			if existing, ok := localToHash[local]; ok && existing != family.Key {
				log.Fatalf("[ERROR] adult local part %s maps to two families", local)
			}
			localToHash[local] = family.Key
		}
	}

	svc, err := drive.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(drive.DriveScope))
	if err != nil {
		log.Fatalf("[ERROR] drive client: %v", err)
	}
	drives, err := svc.Drives.List().Do()
	if err != nil {
		log.Fatalf("[ERROR] list shared drives: %v", err)
	}
	if len(drives.Drives) != 1 {
		log.Fatalf("[ERROR] expected one shared drive, found %d", len(drives.Drives))
	}
	folderList, err := svc.Files.List().
		Q(fmt.Sprintf("name = 'families' and '%s' in parents and mimeType = '%s' and trashed = false", drives.Drives[0].Id, folderMime)).
		SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
		Fields("files(id)").Do()
	if err != nil {
		log.Fatalf("[ERROR] find families folder: %v", err)
	}
	if len(folderList.Files) != 1 {
		log.Fatalf("[ERROR] expected one families folder, found %d", len(folderList.Files))
	}
	folderID := folderList.Files[0].Id

	renamed, kept, unknown := 0, 0, 0
	token := ""
	for {
		call := svc.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
			SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
			Fields("nextPageToken, files(id, name, mimeType)").PageSize(1000)
		if token != "" {
			call = call.PageToken(token)
		}
		list, err := call.Do()
		if err != nil {
			log.Fatalf("[ERROR] list families folder: %v", err)
		}
		for _, f := range list.Files {
			if f.MimeType == folderMime {
				continue
			}
			ext := path.Ext(f.Name)
			base := strings.TrimSuffix(f.Name, ext)
			kind := ""
			for _, k := range []string{"-photo", "-pronunciation"} {
				if strings.HasSuffix(base, k) {
					kind = k
				}
			}
			if kind == "" {
				log.Printf("[ERROR] unrecognized file name %q, leaving it", f.Name)
				unknown++
				continue
			}
			local := strings.TrimSuffix(base, kind)
			hash, ok := localToHash[local]
			if !ok {
				if _, isCurrent := model.Families[local]; isCurrent {
					kept++
					continue
				}
				log.Printf("[ERROR] file %q matches no family adult, leaving it", f.Name)
				unknown++
				continue
			}
			newName := hash + kind + ext
			_, err := svc.Files.Update(f.Id, &drive.File{Name: newName}).SupportsAllDrives(true).Do()
			if err != nil {
				log.Fatalf("[ERROR] rename %q to %q: %v", f.Name, newName, err)
			}
			log.Printf("renamed %q -> %q", f.Name, newName)
			renamed++
		}
		if list.NextPageToken == "" {
			break
		}
		token = list.NextPageToken
	}
	log.Printf("done: %d renamed, %d already keyed by hash, %d left untouched", renamed, kept, unknown)
}
