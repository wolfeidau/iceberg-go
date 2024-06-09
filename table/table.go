// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package table

import (
	"fmt"
	"reflect"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/io"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
)

type Identifier = []string

type Table struct {
	identifier       Identifier
	metadata         Metadata
	metadataLocation string
	fs               io.IO
}

func (t Table) Equals(other Table) bool {
	return slices.Equal(t.identifier, other.identifier) &&
		t.metadataLocation == other.metadataLocation &&
		reflect.DeepEqual(t.metadata, other.metadata)
}

func (t Table) Identifier() Identifier   { return t.identifier }
func (t Table) Metadata() Metadata       { return t.metadata }
func (t Table) MetadataLocation() string { return t.metadataLocation }
func (t Table) FS() io.IO                { return t.fs }

func (t Table) Schema() *iceberg.Schema              { return t.metadata.CurrentSchema() }
func (t Table) Spec() iceberg.PartitionSpec          { return t.metadata.PartitionSpec() }
func (t Table) SortOrder() SortOrder                 { return t.metadata.SortOrder() }
func (t Table) Properties() iceberg.Properties       { return t.metadata.Properties() }
func (t Table) Location() string                     { return t.metadata.Location() }
func (t Table) CurrentSnapshot() *Snapshot           { return t.metadata.CurrentSnapshot() }
func (t Table) SnapshotByID(id int64) *Snapshot      { return t.metadata.SnapshotByID(id) }
func (t Table) SnapshotByName(name string) *Snapshot { return t.metadata.SnapshotByName(name) }
func (t Table) Schemas() map[int]*iceberg.Schema {
	m := make(map[int]*iceberg.Schema)
	for _, s := range t.metadata.Schemas() {
		m[s.ID] = s
	}
	return m
}

func New(ident Identifier, meta Metadata, location string, fs io.IO) *Table {
	return &Table{
		identifier:       ident,
		metadata:         meta,
		metadataLocation: location,
		fs:               fs,
	}
}

func NewFromLocation(ident Identifier, metalocation string, fsys io.IO) (*Table, error) {
	var meta Metadata

	if rf, ok := fsys.(io.ReadFileIO); ok {
		data, err := rf.ReadFile(metalocation)
		if err != nil {
			return nil, err
		}

		if meta, err = ParseMetadataBytes(data); err != nil {
			return nil, err
		}
	} else {
		f, err := fsys.Open(metalocation)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		if meta, err = ParseMetadata(f); err != nil {
			return nil, err
		}
	}
	return New(ident, meta, metalocation, fsys), nil
}

type TableBuilder struct {
	ident            Identifier
	schema           *iceberg.Schema
	partitionSpec    iceberg.PartitionSpec
	sortOrder        SortOrder
	location         string
	metadataLocation string // this path is relative to the location
	properties       iceberg.Properties
}

// NewTableBuilder creates a new TableBuilder for building a Table.
//
// The ident, schema and location parameters are required to create a Table, others
// can be specified with the corresponding builder methods.
func NewTableBuilder(ident Identifier, schema *iceberg.Schema, location, metadataLocation string) *TableBuilder {
	return &TableBuilder{
		ident:            ident,
		schema:           schema,
		location:         location,
		metadataLocation: metadataLocation,
		properties:       make(iceberg.Properties),
	}
}

// WithPartitionSpec sets the partition spec for the table. The partition spec defines how data is partitioned in the table.
func (b *TableBuilder) WithPartitionSpec(spec iceberg.PartitionSpec) *TableBuilder {
	b.partitionSpec = spec
	return b
}

// WithSortOrder sets the sort order for the table. The sort order defines how data is sorted in the table.
func (b *TableBuilder) WithSortOrder(sortOrder SortOrder) *TableBuilder {
	b.sortOrder = sortOrder
	return b
}

func (b *TableBuilder) WithProperties(properties iceberg.Properties) *TableBuilder {
	b.properties = properties
	return b
}

func (b *TableBuilder) Build() (*Table, error) {
	tableUUID := uuid.New()

	// TODO: we need to "freshen" the sequences in the schema, partition spec, and sort order

	metadata, err := NewMetadataV2(b.schema, b.partitionSpec, b.sortOrder, b.location, tableUUID, b.properties)
	if err != nil {
		return nil, err
	}

	// location = s3://<bucket>/<prefix>
	fs, err := io.LoadFS(map[string]string{}, b.location)
	if err != nil {
		return nil, fmt.Errorf("failed to load fs: %w", err)
	}

	return &Table{
		identifier:       b.ident,
		metadata:         metadata,
		metadataLocation: b.metadataLocation,
		fs:               fs,
	}, nil
}

// GenerateMetadataFileName generates a filename for a table metadata file based on the provided table version.
// The filename is in the format "<V>-<random-uuid>.metadata.json", where the V is a 5-digit zero-padded non-negative integer
// and the UUID is a randomly generated UUID string.
//
// If the provided version is negative, an error is returned.
func GenerateMetadataFileName(newVersion int) (string, error) {
	if newVersion < 0 {
		return "", fmt.Errorf("invalid table version: %d must be a non-negative integer", newVersion)
	}

	return fmt.Sprintf("%05d-%s.metadata.json", newVersion, uuid.New().String()), nil
}

func intToPtr(i int) *int {
	return &i
}
