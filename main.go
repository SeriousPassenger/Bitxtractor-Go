package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/btcsuite/btcd/wire"
)

const (
	BlockFile    = "blk00455.dat"
	XORFile      = "xor.dat"
	MainnetMagic = 0xD9B4BEF9
)

type xorReader struct {
	r   io.Reader
	key [8]byte
	pos uint64 // running offset in the file
}

func (x *xorReader) Read(p []byte) (int, error) {
	n, err := x.r.Read(p)
	for i := 0; i < n; i++ {
		p[i] ^= x.key[(x.pos+uint64(i))&7] // (&7) == mod 8
	}
	x.pos += uint64(n)
	return n, err
}

func readBlockFile() error {
	xorKey := [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	// Look for the XOR file. Check if it exists.
	fileInfo, err := os.Stat(XORFile)

	if err != nil {
		log.Println("Error checking the XOR file. Assuming block files are not obfuscated.")
	} else if fileInfo.Size() > 0 {
		log.Println("XOR file is present.")

		file, err := os.Open(XORFile)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = file.Read(xorKey[:])

		if err != nil {
			return err
		}

		if xorKey[0] != 0x00 || xorKey[1] != 0x00 || xorKey[2] != 0x00 || xorKey[3] != 0x00 {
			log.Println("XOR key is not 0x00. Block files are obfuscated.")
		}

		log.Println("XOR key:", hex.EncodeToString(xorKey[:]))
	}

	blockFile, err := os.Open(BlockFile)
	if err != nil {
		return err
	}
	defer blockFile.Close()

	xr := &xorReader{r: bufio.NewReader(blockFile), key: xorKey}

	blockReader := bufio.NewReader(xr)

	for {
		var magic uint32
		if err := binary.Read(blockReader, binary.LittleEndian, &magic); err == io.EOF {
			break
		}
		if magic != MainnetMagic {
			log.Fatalf("bad magic 0x%x at offset %d", magic, xr.pos-4)
		}

		var length uint32 // 4 bytes
		if err := binary.Read(blockReader, binary.LittleEndian, &length); err != nil {
			log.Fatal(err)
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(blockReader, payload); err != nil {
			log.Fatal(err)
		}
		var blk wire.MsgBlock
		if err := blk.Deserialize(bytes.NewReader(payload)); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Block hash: %s, %d tx(s) present in block.\n",
			blk.Header.BlockHash(), len(blk.Transactions))

		// Process transactions
		for i, tx := range blk.Transactions {
			txHash := tx.TxHash()
			fmt.Printf("  Tx %d: %s\n", i, txHash)

			// Display inputs
			fmt.Printf("    Inputs: %d\n", len(tx.TxIn))
			for j, input := range tx.TxIn {
				if j < 3 { // Limit to first 3 inputs to avoid too much output
					prevTxHash := input.PreviousOutPoint.Hash
					prevTxIndex := input.PreviousOutPoint.Index
					fmt.Printf("      Input %d: %s:%d\n", j, prevTxHash, prevTxIndex)
				}
			}

			// Display outputs
			fmt.Printf("    Outputs: %d\n", len(tx.TxOut))
			for k, output := range tx.TxOut {
				if k < 3 { // Limit to first 3 outputs to avoid too much output
					value := float64(output.Value) / 100000000 // Convert from satoshis to BTC
					fmt.Printf("      Output %d: %.8f BTC\n", k, value)
				}
			}

			// Limit to 5 transactions per block to avoid flooding the console
			if i >= 4 {
				fmt.Printf("  ... and %d more transactions\n", len(blk.Transactions)-5)
				break
			}
		}
		fmt.Println()
	}

	return nil
}

func main() {
	err := readBlockFile()
	if err != nil {
		log.Fatal(err)
	}
}
