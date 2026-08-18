package hostcollate

import catalog "github.com/nstranquist/nicos-catalog"

// acceptedRecords keeps only entities the engine Discover path would accept.
func acceptedRecords(records []catalog.Record) []catalog.Record {
	var out []catalog.Record
	for _, record := range records {
		if err := acceptEntity(record.Entity); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out
}

func acceptEntity(entity catalog.Entity) error {
	if err := catalog.ValidateEntityID(entity.ID); err != nil {
		return err
	}
	if entity.Name == "" || entity.Kind == "" {
		return catalog.ErrInvalidEntity
	}
	if !entity.Visibility.Valid() {
		return catalog.ErrInvalidEntity
	}
	for _, ref := range entity.Refs {
		if ref.Kind == "" || ref.Target == "" {
			return catalog.ErrInvalidEntity
		}
	}
	return nil
}
