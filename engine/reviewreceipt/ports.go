package reviewreceipt

import "errors"

// The ports are what the domain asks of the world. It asks for no file, no process and no
// git: where review transactions live, where a change or a feature keeps its persisted receipts, and what
// the openspec/changes directory holds are all answered by an adapter that the composition
// root hands in (engine/reviewreceipt/fsstore is the one made of files and git). Each
// adapter is built over the project the hook was started for, so the domain never names it.
//
// The words an adapter uses for a failure are its own ("read <path>: ..."); the domain adds
// the name of the package in front, so a person reads where a failure came from.

// Store names one review-transaction store: a place the review tool keeps its transactions,
// one directory per lineage. The domain treats it as an opaque name that
// TransactionStores gives out and ReceiptSource takes back.
type Store string

// Document is one candidate receipt a store holds: the bytes of a file of a lineage, and
// which shape of file it is. The bytes are exactly what the file held.
type Document struct {
	Shape Shape
	Data  []byte
	// Origin names where the document was read, as the source names it (a file path for a
	// file store). The domain only reports it, so a person can find the document.
	Origin string
}

// TransactionStores says where the review transactions of the project are.
type TransactionStores interface {
	// Stores lists the stores that hold the project's review transactions, without
	// duplicates: the working tree's own first, then the one shared by every working tree
	// of the repository when it is another. It fails when the project is not in a
	// repository.
	Stores() ([]Store, error)
}

// ReceiptSource reads what a store holds.
type ReceiptSource interface {
	// Documents returns every candidate receipt the store holds, in a stable order:
	// lineage by lineage in name order, the legacy receipt before the lifecycle state. A
	// store that does not exist holds nothing. A file that cannot be read is not a
	// document. It fails when the store itself cannot be read.
	Documents(store Store) ([]Document, error)
}

// ErrNotPersisted is what ReceiptSink.Read reports (wrapped) for a receipt that has not
// been persisted.
var ErrNotPersisted = errors.New("no such persisted receipt")

// DestinationKind says which kind of folder a Destination names.
type DestinationKind int

const (
	// DestinationChange is the review-receipts folder of an SDD change.
	DestinationChange DestinationKind = iota + 1
	// DestinationFeature is the review-receipts folder of an ODD feature, beside the feature's
	// task document.
	DestinationFeature
)

// Destination is the folder a receipt is persisted in: the review-receipts folder of a change,
// or the one of a feature. Where each is on disk is the sink's to know; the domain only names
// it. Build one with InChange or InFeature; the zero value names no folder.
type Destination struct {
	Kind DestinationKind
	// Name is the change name or the feature name, one path component (CheckPathComponent).
	Name string
}

// InChange is the review-receipts folder of the change.
func InChange(change string) Destination { return Destination{Kind: DestinationChange, Name: change} }

// InFeature is the review-receipts folder of the feature.
func InFeature(feature string) Destination {
	return Destination{Kind: DestinationFeature, Name: feature}
}

// kindName is what the destination's name is for, as an UnsafeNameError says it.
func (d Destination) kindName() string {
	if d.Kind == DestinationFeature {
		return kindFeature
	}
	return kindChange
}

// FeatureCatalog lists the features the project keeps persisted receipts for.
type FeatureCatalog interface {
	// Features lists the feature folders that hold persisted receipts, by name. A project
	// with none has none; a folder that is there and cannot be listed is an error, never an
	// absence, since the hook counts the receipts persisted in them.
	Features() ([]string, error)
}

// ReceiptSink keeps receipts where the project versions them: the review-receipts folder of a
// change or of a feature (a Destination), and it lists the feature folders it keeps
// (FeatureCatalog). The destination's name and the name every method is given are single safe
// path components (CheckPathComponent): the Service checks them before it asks, so an adapter
// may join them into a path without checking them again.
type ReceiptSink interface {
	FeatureCatalog
	// Read returns the bytes persisted under name in the destination, or an error satisfying
	// errors.Is(err, ErrNotPersisted) when there are none.
	Read(dest Destination, name string) ([]byte, error)
	// Write persists data under name in the destination, atomically and creating the folder
	// if it must. The domain reads first and never asks to replace a file whose bytes differ.
	Write(dest Destination, name string, data []byte) error
	// Location says where a receipt is, or would be, for a message and for Captured.Path.
	// It touches nothing.
	Location(dest Destination, name string) string
}

// ChangeCatalog lists the changes the project has under openspec/changes.
type ChangeCatalog interface {
	// Changes lists the directories under openspec/changes, by name. A project with no
	// such directory has none.
	Changes() ([]string, error)
	// HasArtifact reports whether the change holds a file or a directory called name. A change
	// that cannot be looked at is an error, never an absence: the hook counts the changes that
	// hold an artifact, and one dropped in silence would let an acknowledgement through.
	HasArtifact(change, name string) (bool, error)
}

// Ports are the four things a Service is built over.
type Ports struct {
	Stores  TransactionStores
	Source  ReceiptSource
	Sink    ReceiptSink
	Changes ChangeCatalog
}
