package metadata

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

type collectionInfo struct {
	head pageAddress
	tail pageAddress
}

type indexNode struct {
	address pageAddress
	next    pageAddress
	data    pageAddress
	key     string
}

func (r *playniteReader) readCollections(address pageAddress) (map[string]collectionInfo, error) {
	if !address.valid(r.logicalSize) {
		return nil, errPlayniteCorrupt
	}
	page, err := r.page(address.page)
	if err != nil || page.pageType != playniteCollectionPageType {
		return nil, errPlayniteCorrupt
	}
	position := playniteBaseHeaderSize
	name, next, ok := readLiteString32(page.bytes, position)
	if !ok || !strings.EqualFold(name, "games") {
		return nil, errPlayniteCorrupt
	}
	position = next
	if position+8+4 > playnitePageSize {
		return nil, errPlayniteCorrupt
	}
	position += 8 + 4 // DocumentCount and FreeDataPageID.
	var primary collectionInfo
	for slot := 0; slot < 16; slot++ {
		field, after, ok := readLiteString32(page.bytes, position)
		if !ok || after+1+6+6+4 > playnitePageSize {
			return nil, errPlayniteCorrupt
		}
		position = after
		unique := page.bytes[position] != 0
		position++
		head := readPageAddress(page.bytes[position:])
		position += 6
		tail := readPageAddress(page.bytes[position:])
		position += 6
		position += 4 // FreeIndexPageID.
		fieldName := field
		if equal := strings.IndexByte(fieldName, '='); equal >= 0 {
			fieldName = fieldName[:equal]
		}
		if slot == 0 {
			if fieldName != "_id" || !unique || !head.valid(r.logicalSize) || !tail.valid(r.logicalSize) {
				return nil, errPlayniteCorrupt
			}
			primary = collectionInfo{head: head, tail: tail}
		}
	}
	return map[string]collectionInfo{"games": primary}, nil
}

func readLiteString32(data []byte, at int) (string, int, bool) {
	if at < 0 || at+4 > len(data) {
		return "", 0, false
	}
	length := int64(int32(binary.LittleEndian.Uint32(data[at:])))
	if length < 0 || length > 4096 || length > int64(len(data)-at-4) {
		return "", 0, false
	}
	value := string(data[at+4 : at+4+int(length)])
	if strings.IndexByte(value, 0) >= 0 {
		return "", 0, false
	}
	return value, at + 4 + int(length), true
}

func (r *playniteReader) readGames(collection collectionInfo) ([]PlayniteGame, error) {
	nodes, err := r.readLevelZero(collection)
	if err != nil {
		return nil, err
	}
	games := make([]PlayniteGame, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		doc, err := r.readDataDocument(node.data)
		if err != nil {
			return nil, err
		}
		game, err := gameFromDocument(doc)
		if err != nil || node.key != game.PlayniteGUID {
			return nil, errPlayniteCorrupt
		}
		if _, duplicate := seen[game.PlayniteGUID]; duplicate {
			return nil, errPlayniteCorrupt
		}
		seen[game.PlayniteGUID] = struct{}{}
		games = append(games, game)
	}
	sort.Slice(games, func(i, j int) bool {
		if strings.EqualFold(games[i].Name, games[j].Name) {
			return games[i].PlayniteGUID < games[j].PlayniteGUID
		}
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})
	return games, nil
}

func (r *playniteReader) readLevelZero(collection collectionInfo) ([]indexNode, error) {
	head, err := r.readIndexNode(collection.head)
	if err != nil {
		return nil, err
	}
	current := head.next
	seen := make(map[pageAddress]struct{})
	nodes := make([]indexNode, 0, 128)
	for current != collection.tail {
		if current.empty() || !current.valid(r.logicalSize) {
			return nil, errPlayniteCorrupt
		}
		if _, exists := seen[current]; exists || len(seen) >= maxPlayniteGames {
			return nil, fmt.Errorf("%w: index chain cycle", errPlayniteCorrupt)
		}
		seen[current] = struct{}{}
		node, err := r.readIndexNode(current)
		if err != nil || node.data.empty() || !node.data.valid(r.logicalSize) || node.key == "" {
			return nil, errPlayniteCorrupt
		}
		nodes = append(nodes, node)
		current = node.next
	}
	if _, err := r.readIndexNode(collection.tail); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (r *playniteReader) readIndexNode(address pageAddress) (indexNode, error) {
	page, err := r.page(address.page)
	if err != nil || page.pageType != playniteIndexPageType {
		return indexNode{}, errPlayniteCorrupt
	}
	position := playniteBaseHeaderSize
	for item := 0; item < int(page.itemCount); item++ {
		if position+2+1+1+6+6+2+1 > playnitePageSize {
			return indexNode{}, errPlayniteCorrupt
		}
		index := binary.LittleEndian.Uint16(page.bytes[position:])
		position += 2
		levels := int(page.bytes[position])
		position++
		if levels < 1 || levels > 32 {
			return indexNode{}, errPlayniteCorrupt
		}
		position++    // collection index slot
		position += 6 // PrevNode
		position += 6 // NextNode
		keyLength := int(binary.LittleEndian.Uint16(page.bytes[position:]))
		position += 2
		if keyLength < 0 || position+1+keyLength+6+levels*12 > playnitePageSize {
			return indexNode{}, errPlayniteCorrupt
		}
		keyType := page.bytes[position]
		position++
		keyBytes := page.bytes[position : position+keyLength]
		position += keyLength
		data := readPageAddress(page.bytes[position:])
		position += 6
		var levelZeroNext pageAddress
		for level := 0; level < levels; level++ {
			position += 6 // Prev[level]
			next := readPageAddress(page.bytes[position:])
			position += 6
			if level == 0 {
				levelZeroNext = next
			}
		}
		if index != address.slot {
			continue
		}
		node := indexNode{address: address, next: levelZeroNext, data: data}
		if keyType == 11 && keyLength == 16 {
			guid, ok := guidValue(bsonValue{kind: bsonBinary, subtype: 4, bytes: append([]byte(nil), keyBytes...)})
			if !ok {
				return indexNode{}, errPlayniteCorrupt
			}
			node.key = guid
		} else if keyType != 0 && keyType != 14 {
			return indexNode{}, fmt.Errorf("%w: unsupported primary key type", errPlayniteCorrupt)
		}
		return node, nil
	}
	return indexNode{}, errPlayniteCorrupt
}

func (r *playniteReader) readDataDocument(address pageAddress) (*bsonDocument, error) {
	page, err := r.page(address.page)
	if err != nil || page.pageType != playniteDataPageType {
		return nil, errPlayniteCorrupt
	}
	position := playniteBaseHeaderSize
	for item := 0; item < int(page.itemCount); item++ {
		if position+2+4+2 > playnitePageSize {
			return nil, errPlayniteCorrupt
		}
		index := binary.LittleEndian.Uint16(page.bytes[position:])
		position += 2
		extendPageID := binary.LittleEndian.Uint32(page.bytes[position:])
		position += 4
		length := int(binary.LittleEndian.Uint16(page.bytes[position:]))
		position += 2
		if length < 0 || position+length > playnitePageSize {
			return nil, errPlayniteCorrupt
		}
		inline := append([]byte(nil), page.bytes[position:position+length]...)
		position += length
		if index != address.slot {
			continue
		}
		data := inline
		if extendPageID != ^uint32(0) {
			data, err = r.readExtendData(extendPageID)
			if err != nil {
				return nil, err
			}
		}
		if len(data) < 5 || len(data) > maxBSONBytes {
			return nil, errPlayniteCorrupt
		}
		doc, consumed, err := parseBSONDocument(data)
		if err != nil || consumed != len(data) {
			return nil, errPlayniteCorrupt
		}
		return doc, nil
	}
	return nil, errPlayniteCorrupt
}

func (r *playniteReader) readExtendData(first uint32) ([]byte, error) {
	var data []byte
	seen := make(map[uint32]struct{})
	current := first
	for current != ^uint32(0) {
		if _, exists := seen[current]; exists || len(seen) >= maxPlaynitePages {
			return nil, errPlayniteCorrupt
		}
		seen[current] = struct{}{}
		page, err := r.page(current)
		if err != nil || page.pageType != playniteExtendPageType ||
			int(page.itemCount) > playnitePageSize-playniteBaseHeaderSize {
			return nil, errPlayniteCorrupt
		}
		if len(data)+int(page.itemCount) > maxBSONBytes {
			return nil, errPlayniteCorrupt
		}
		data = append(data, page.bytes[playniteBaseHeaderSize:playniteBaseHeaderSize+int(page.itemCount)]...)
		current = page.next
	}
	return data, nil
}

func gameFromDocument(doc *bsonDocument) (PlayniteGame, error) {
	if doc == nil {
		return PlayniteGame{}, errPlayniteCorrupt
	}
	for _, required := range []string{"_id", "Name", "GameId", "PluginId", "PlatformIds", "SourceId", "Hidden"} {
		if doc.hasDuplicate(required) {
			return PlayniteGame{}, fmt.Errorf("%w: duplicate top-level %s", errPlayniteCorrupt, required)
		}
	}
	idValue, ok := doc.field("_id")
	if !ok {
		return PlayniteGame{}, fmt.Errorf("%w: game has no top-level _id", errPlayniteCorrupt)
	}
	id, ok := guidValue(idValue)
	if !ok || id == "" {
		return PlayniteGame{}, fmt.Errorf("%w: game id is not a Guid", errPlayniteCorrupt)
	}
	nameValue, ok := doc.field("Name")
	name, nameOK := nameValue.stringValue()
	if !ok || !nameOK || name == "" {
		return PlayniteGame{}, fmt.Errorf("%w: game has no top-level Name", errPlayniteCorrupt)
	}
	game := PlayniteGame{PlayniteGUID: id, Name: name}
	if value, ok := doc.field("GameId"); ok {
		var valid bool
		game.GameID, valid = value.stringValue()
		if !valid {
			return PlayniteGame{}, fmt.Errorf("%w: GameId is not a string", errPlayniteCorrupt)
		}
	}
	if value, ok := doc.field("PluginId"); ok {
		if value.kind == bsonBinary {
			var valid bool
			game.PluginID, valid = guidValue(value)
			if !valid {
				return PlayniteGame{}, fmt.Errorf("%w: PluginId is not a Guid", errPlayniteCorrupt)
			}
		} else if value.kind != bsonNull {
			return PlayniteGame{}, fmt.Errorf("%w: PluginId has invalid type", errPlayniteCorrupt)
		}
	}
	if value, ok := doc.field("SourceId"); ok {
		if value.kind == bsonBinary {
			var valid bool
			game.SourceID, valid = guidValue(value)
			if !valid {
				return PlayniteGame{}, fmt.Errorf("%w: SourceId is not a Guid", errPlayniteCorrupt)
			}
		} else if value.kind != bsonNull {
			return PlayniteGame{}, fmt.Errorf("%w: SourceId has invalid type", errPlayniteCorrupt)
		}
	}
	if value, ok := doc.field("Hidden"); ok {
		if value.kind != bsonBool {
			return PlayniteGame{}, fmt.Errorf("%w: Hidden is not a boolean", errPlayniteCorrupt)
		}
		game.Hidden = value.boolean
	}
	if value, ok := doc.field("PlatformIds"); ok {
		if value.kind != bsonArrayKind || value.doc == nil {
			return PlayniteGame{}, fmt.Errorf("%w: PlatformIds is not an array", errPlayniteCorrupt)
		}
		for _, field := range value.doc.fields {
			id, valid := guidValue(field.value)
			if !valid {
				return PlayniteGame{}, fmt.Errorf("%w: PlatformIds item is not a Guid", errPlayniteCorrupt)
			}
			game.PlatformIDs = append(game.PlatformIDs, id)
		}
	}
	for _, field := range doc.fields {
		lower := strings.ToLower(field.name)
		if lower != "coverimage" && lower != "backgroundimage" && lower != "icon" {
			continue
		}
		value, valid := field.value.stringValue()
		if valid && value != "" {
			game.Media = append(game.Media, PlayniteMediaReference{Kind: lower, Path: value})
		}
	}
	sort.Slice(game.PlatformIDs, func(i, j int) bool { return game.PlatformIDs[i] < game.PlatformIDs[j] })
	sort.Slice(game.Media, func(i, j int) bool {
		if game.Media[i].Kind != game.Media[j].Kind {
			return game.Media[i].Kind < game.Media[j].Kind
		}
		return game.Media[i].Path < game.Media[j].Path
	})
	return game, nil
}
