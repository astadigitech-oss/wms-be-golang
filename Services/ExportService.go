package services

import (
	"database/sql"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"time"

	"github.com/xuri/excelize/v2"
)

//======================== Repair Document (damaged, non) ===========================
func WriteAllProductRepair(f *excelize.File, sheet, repairType string) error {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}

	db := config.DB
	const chunkSize = 500

	streamWriter, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}

	headers := []interface{}{
		"Source",
		"Document Status",
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"Name Product",
		"Category Product",
		"Qty Product",
		"Old Price Product",
		"New Price Product",
		"Date In",
		"Description",
		"Color Tag",
		"Discount",
		"Created At",
	}

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

	lastID := 0
	rowIndex := 2

	for {
		query := `
			SELECT 
				p.id,
				CASE 
					WHEN p.quality = 'migrate' THEN 'migrate'
					WHEN p.location_type = 'main' THEN 'display'
					ELSE 'staging' 
				END AS source,
				COALESCE(rd.status, 'N/A') AS document_status,
				COALESCE(p.code_document, 'NULL') AS code_document,
				COALESCE(p.old_barcode_product, 'NULL') AS old_barcode_product,
				p.barcode,
				p.name,
				COALESCE(c.name_category, 'NULL') AS category,
				p.quantity,
				p.old_price_product,
				p.price,
				COALESCE(p.quality_text, 'NULL') AS quality_text,
				COALESCE(ct.name_color, 'NULL') AS color_tag,
				p.discount,
				p.created_at
			FROM products p
			LEFT JOIN categories c 
				ON c.id = p.category_id
			LEFT JOIN color_tags ct 
				ON ct.id = p.tag_color_id
			JOIN repair_document_items rdi 
				ON rdi.product_id = p.id
			JOIN repair_documents rd 
				ON rd.id = rdi.repair_document_id
			WHERE p.id > ?
			AND rd.type_document = ?
			ORDER BY p.id ASC
			LIMIT ?
		`

		rows, err := db.Raw(query, lastID, repairType, chunkSize).Rows()
		if err != nil {
			return err
		}

		count := 0

		for rows.Next() {
			var (
				idProd int
				source, docStatus, code string
				oldBarcode, newBarcode string
				name, category, qualityText, colorTag string
				qty int
				oldPrice, newPrice float64
                discount sql.NullFloat64
				createdAt time.Time
			)

			err := rows.Scan(
				&idProd,
				&source,
				&docStatus,
				&code,
				&oldBarcode,
				&newBarcode,
				&name,
				&category,
				&qty,
				&oldPrice,
				&newPrice,
				&qualityText,
				&colorTag,
				&discount,
				&createdAt,
			)
			if err != nil {
				rows.Close()
				return err
			}

			dateIn := createdAt.In(loc).Format("2006-01-02")
            var discountValue float64
            if discount.Valid {
                discountValue = discount.Float64
            } else {
                discountValue = 0 // atau sesuai kebutuhan
            }

			row := []interface{}{
				source,
				docStatus,
				code,
				" " + oldBarcode,
				" " + newBarcode,
				name,
				category,
				qty,
				oldPrice,
				newPrice,
				dateIn,
				qualityText,
				colorTag,
				discountValue,
				createdAt.In(loc).Format("2006-01-02 15:04"),
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := streamWriter.SetRow(cell, row); err != nil {
				rows.Close()
				return err
			}

			lastID = idProd
			rowIndex++
			count++
		}

		rows.Close()

		// Kalau tidak ada data lagi, stop loop
		if count == 0 {
			break
		}
	}

	return streamWriter.Flush()
}

func WriteRepairSummaryStream(f *excelize.File, sheet, repairType string) error {
    db := config.DB
    streamWriter, err := f.NewStreamWriter(sheet)
    if err != nil {
        return err
    }

    headers := []interface{}{
        "Code Document",
        "User Name",
        "Total Product",
        "Total New Price",
        "Total Old Price",
        "Document Status",
        "Created At",
        "Updated At",
    }

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

    query := `
        SELECT 
            rd.code_document,
            COALESCE(u.name, 'N/A'),
            rd.total_product,
            rd.total_new_price,
            rd.total_old_price,
            rd.status,
            rd.created_at,
            rd.updated_at
        FROM repair_documents rd
        LEFT JOIN users u ON u.id = rd.user_id
        WHERE rd.type_document = ?
        ORDER BY rd.created_at DESC
    `

    rows, err := db.Raw(query, repairType).Rows()
    if err != nil {
        return err
    }
    defer rows.Close()

    rowIndex := 2

    for rows.Next() {

        var (
            code, userName, status string
            totalProduct int
            totalNew, totalOld float64
            createdAt, updatedAt time.Time
        )

        rows.Scan(
            &code,
            &userName,
            &totalProduct,
            &totalNew,
            &totalOld,
            &status,
            &createdAt,
            &updatedAt,
        )

        row := []interface{}{
            code,
            userName,
            totalProduct,
            totalNew,
            totalOld,
            status,
            createdAt.Format("2006-01-02 15:04"),
            updatedAt.Format("2006-01-02 15:04"),
        }

        cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
        if err := streamWriter.SetRow(cell, row); err != nil {
            return err
        }

        rowIndex++
    }

    return streamWriter.Flush()
}

//======================== Scrap ===========================
//per document
func WriteProductScrapQcd(f *excelize.File, sheet string, doc_id uint64) error {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}

	db := config.DB
	const chunkSize = 500

	streamWriter, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}

	headers := []interface{}{
		"Source",
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"Name Product",
		"Category Product",
		"Qty Product",
		"Old Price Product",
		"New Price Product",
		"Date In",
		"Status",
		"Quality Description",
		"Color Tag",
		"Discount",
		"Created At",
	}

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

	lastID := 0
	rowIndex := 2

	for {
		query := `
			SELECT 
				p.id,
				CASE 
					WHEN p.quality = 'migrate' THEN 'migrate'
					WHEN p.location_type = 'main' THEN 'display'
					ELSE 'staging' 
				END AS source,
				COALESCE(sd.status, 'N/A') AS document_status,
				COALESCE(p.code_document, 'NULL') AS code_document,
				COALESCE(p.old_barcode_product, 'NULL') AS old_barcode_product,
				p.barcode,
				p.name,
				COALESCE(c.name_category, 'NULL') AS category,
				p.quantity,
				p.old_price_product,
				p.price,
				p.status,
				COALESCE(p.quality_text, 'NULL') AS quality_text,
				COALESCE(ct.name_color, 'NULL') AS color_tag,
				p.discount,
				p.created_at
			FROM products p
			LEFT JOIN categories c 
				ON c.id = p.category_id
			LEFT JOIN color_tags ct 
				ON ct.id = p.tag_color_id
			JOIN scrap_items si 
				ON si.product_id = p.id
			JOIN scrap_documents sd 
				ON sd.id = si.scrap_document_id
			WHERE p.id > ? AND sd.id = ?
			ORDER BY p.id ASC
			LIMIT ?
		`

		rows, err := db.Raw(query, lastID, doc_id, chunkSize).Rows()
		if err != nil {
			return err
		}

		count := 0

		for rows.Next() {
			var (
				idProd int
				source, docStatus, status, code string
				oldBarcode, newBarcode string
				name, category, qualityText, colorTag string
				qty int
				oldPrice, newPrice float64
                discount sql.NullFloat64
				createdAt time.Time
			)

			err := rows.Scan(
				&idProd,
				&source,
				&docStatus,
				&code,
				&oldBarcode,
				&newBarcode,
				&name,
				&category,
				&qty,
				&oldPrice,
				&newPrice,
                &status,
				&qualityText,
				&colorTag,
				&discount,
				&createdAt,
			)
			if err != nil {
				rows.Close()
				return err
			}

			dateIn := createdAt.In(loc).Format("2006-01-02")
            var discountValue float64
            if discount.Valid {
                discountValue = discount.Float64
            } else {
                discountValue = 0 // atau sesuai kebutuhan
            }

			row := []interface{}{
				source,
				code,
				" " + oldBarcode,
				" " + newBarcode,
				name,
				category,
				qty,
				oldPrice,
				newPrice,
				dateIn,
                status,
				qualityText,
				colorTag,
				discountValue,
				createdAt.In(loc).Format("2006-01-02 15:04"),
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := streamWriter.SetRow(cell, row); err != nil {
				rows.Close()
				return err
			}

			lastID = idProd
			rowIndex++
			count++
		}

		rows.Close()

		// Kalau tidak ada data lagi, stop loop
		if count == 0 {
			break
		}
	}

	return streamWriter.Flush()
}

func WriteScrapQcdSummaryDocument(f *excelize.File, sheet string, document models.ScrapDocument) error {
    loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}

    streamWriter, err := f.NewStreamWriter(sheet)
    if err != nil {
        return err
    }

    headers := []interface{}{
        "Code Document",
        "User Name",
        "Total Product",
        "Total New Price",
        "Total Old Price",
        "Document Status",
        "Created At",
        "Updated At",
    }

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

    row := []interface{}{
        document.CodeDocument,
        document.User.Name,
        document.TotalProduct,
        document.TotalNewPrice,
        document.TotalOldPrice,
        document.Status,
        document.CreatedAt.In(loc).Format("2006-01-02 15:04"),
        document.UpdatedAt.In(loc).Format("2006-01-02 15:04"),
    }

    cellRow, _ := excelize.CoordinatesToCellName(1, 2)
    if err := streamWriter.SetRow(cellRow, row); err != nil {
        return err
    }

    return streamWriter.Flush()
}

//summary
func WriteAllProductScrapQcd(f *excelize.File, sheet string) error {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}

	db := config.DB
	const chunkSize = 500

	streamWriter, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}

	headers := []interface{}{
		"Source",
		"Document Status",
		"Code Document",
		"Old Barcode Product",
		"New Barcode Product",
		"Name Product",
		"Category Product",
		"Qty Product",
		"Old Price Product",
		"New Price Product",
		"Date In",
		"Description",
		"Color Tag",
		"Discount",
		"Created At",
	}

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

	lastID := 0
	rowIndex := 2

	for {
		query := `
			SELECT 
				p.id,
				CASE 
					WHEN p.quality = 'migrate' THEN 'migrate'
					WHEN p.location_type = 'main' THEN 'display'
					ELSE 'staging' 
				END AS source,
				COALESCE(sd.status, 'N/A') AS document_status,
				COALESCE(p.code_document, 'NULL') AS code_document,
				COALESCE(p.old_barcode_product, 'NULL') AS old_barcode_product,
				p.barcode,
				p.name,
				COALESCE(c.name_category, 'NULL') AS category,
				p.quantity,
				p.old_price_product,
				p.price,
				COALESCE(p.quality_text, 'NULL') AS quality_text,
				COALESCE(ct.name_color, 'NULL') AS color_tag,
				p.discount,
				p.created_at
			FROM products p
			LEFT JOIN categories c 
				ON c.id = p.category_id
			LEFT JOIN color_tags ct 
				ON ct.id = p.tag_color_id
			JOIN scrap_items si 
				ON si.product_id = p.id
			JOIN scrap_documents sd 
				ON sd.id = si.scrap_document_id
			WHERE p.id > ?
			ORDER BY p.id ASC
			LIMIT ?
		`

		rows, err := db.Raw(query, lastID, chunkSize).Rows()
		if err != nil {
			return err
		}

		count := 0

		for rows.Next() {
			var (
				idProd int
				source, docStatus, code string
				oldBarcode, newBarcode string
				name, category, qualityText, colorTag string
				qty int
				oldPrice, newPrice float64
                discount sql.NullFloat64
				createdAt time.Time
			)

			err := rows.Scan(
				&idProd,
				&source,
				&docStatus,
				&code,
				&oldBarcode,
				&newBarcode,
				&name,
				&category,
				&qty,
				&oldPrice,
				&newPrice,
				&qualityText,
				&colorTag,
				&discount,
				&createdAt,
			)
			if err != nil {
				rows.Close()
				return err
			}

			dateIn := createdAt.In(loc).Format("2006-01-02")
            var discountValue float64
            if discount.Valid {
                discountValue = discount.Float64
            } else {
                discountValue = 0 // atau sesuai kebutuhan
            }

			row := []interface{}{
				source,
				docStatus,
				code,
				" " + oldBarcode,
				" " + newBarcode,
				name,
				category,
				qty,
				oldPrice,
				newPrice,
				dateIn,
				qualityText,
				colorTag,
				discountValue,
				createdAt.In(loc).Format("2006-01-02 15:04"),
			}

			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := streamWriter.SetRow(cell, row); err != nil {
				rows.Close()
				return err
			}

			lastID = idProd
			rowIndex++
			count++
		}

		rows.Close()

		// Kalau tidak ada data lagi, stop loop
		if count == 0 {
			break
		}
	}

	return streamWriter.Flush()
}

func WriteScrapQcdSummaryAllDocument(f *excelize.File, sheet string) error {
    loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}

    db := config.DB
    streamWriter, err := f.NewStreamWriter(sheet)
    if err != nil {
        return err
    }

    headers := []interface{}{
        "Code Document",
        "User Name",
        "Total Product",
        "Total New Price",
        "Total Old Price",
        "Document Status",
        "Created At",
        "Updated At",
    }

    //set width colom
    startCol,_ := excelize.ColumnNumberToName(1)
    endCol, _ := excelize.ColumnNumberToName(len(headers))

    if err := f.SetColWidth(sheet, startCol, endCol, 20.0); err != nil {
        return err
    }

    //style header
    styleID, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{
            Bold:  true,
            Size:  11,
            Color: "FFFFFF",
        },
        Alignment: &excelize.Alignment{
            Vertical:   "center",
        },
        Fill: excelize.Fill{
            Type:    "pattern",
            Pattern: 1,
            Color:   []string{"4472C4"},
        },
    })

    var headerRow []interface{}
    for _, h := range headers {
        headerRow = append(headerRow, excelize.Cell{
            StyleID: styleID,
            Value:   h,
        })
    }

    cell, _ := excelize.CoordinatesToCellName(1, 1)
    if err := streamWriter.SetRow(cell, headerRow); err != nil {
        return err
    }

    query := `
        SELECT 
            sd.code_document,
            COALESCE(u.name, 'N/A'),
            sd.total_product,
            sd.total_new_price,
            sd.total_old_price,
            sd.status,
            sd.created_at,
            sd.updated_at
        FROM scrap_documents sd
        LEFT JOIN users u ON u.id = sd.user_id
        ORDER BY sd.created_at DESC
    `

    rows, err := db.Raw(query).Rows()
    if err != nil {
        return err
    }
    defer rows.Close()

    rowIndex := 2

    for rows.Next() {

        var (
            code, userName, status string
            totalProduct int
            totalNew, totalOld float64
            createdAt, updatedAt time.Time
        )

        rows.Scan(
            &code,
            &userName,
            &totalProduct,
            &totalNew,
            &totalOld,
            &status,
            &createdAt,
            &updatedAt,
        )

        row := []interface{}{
            code,
            userName,
            totalProduct,
            totalNew,
            totalOld,
            status,
            createdAt.In(loc).Format("2006-01-02 15:04"),
            updatedAt.In(loc).Format("2006-01-02 15:04"),
        }

        cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
        if err := streamWriter.SetRow(cell, row); err != nil {
            return err
        }

        rowIndex++
    }

    return streamWriter.Flush()
}



