package mra

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func mra2rom(root *XMLNode) {
	rbf := root.GetNode("setname")
	xml_rom := root.FindMatch(func(n *XMLNode) bool { return n.name == "rom" && n.GetAttr("index") == "0" })
	if xml_rom == nil || rbf == nil {
		fmt.Printf("Warning: malformed MRA file")
		return
	}
	rombytes := make([]byte,0)
	var zf []*zip.ReadCloser
	for _, each := range strings.Split(xml_rom.GetAttr("zip"), "|") {
		aux := get_zipfile(each)
		if aux != nil {
			zf = append(zf, aux)
		}
	}
	parts2rom(zf, xml_rom, &rombytes)
	fout_name := filepath.Join(os.Getenv("JTROOT"), "rom", rbf.text+".rom") // rbf.text should be shortened to match mra.exe's output
	fout, err := os.Create(fout_name)
	if err != nil {
		fmt.Println(err)
		return
	}
	fout.Write(rombytes)
	fout.Close()
}

func parts2rom(zf []*zip.ReadCloser, n *XMLNode, rb *[]byte) {
	for _, each := range n.children {
		if each.name == "part" {
			fname := each.GetAttr("name")
			if fname == "" {
				// Convert internal text to bytes
                data := make([]byte,0)
				for _, token := range strings.Split(each.text, " \n\t") {
					if token == "" {
						continue
					}
					v, _ := strconv.ParseInt(token, 16, 16)
					data = append(data, byte(v&0xff))
				}
                rep,_ := strconv.ParseInt(each.GetAttr("repeat"),0,32)
                if rep==0 {
                    rep = 1
                }
                for ;rep>0;rep-- {
                    *rb = append( *rb, data... )
                }
			} else {
				*rb = append(*rb, readrom(zf, each)...)
			}
		}
	}
}

func readrom(allzips []*zip.ReadCloser, n *XMLNode) (rdin []byte) {
	crc := strings.ToLower(n.GetAttr("crc"))
	var f *zip.File
lookup:
	for _, each := range allzips {
		for _, file := range each.File {
			if fmt.Sprintf("%08x",file.CRC32) == crc {
				f = file
				break lookup
			}
		}
	}
	if f == nil {
        fmt.Printf("Warning: cannot find file %s (%s) in zip\n",n.GetAttr("name"),n.GetAttr("crc"))
        return nil
    }
	offset, _ := strconv.ParseInt(n.GetAttr("offset"), 0, 32)
	lenght, _ := strconv.ParseInt(n.GetAttr("length"), 0, 32)
	zpart, _ := f.Open()
	alldata := make([]byte, f.UncompressedSize64)
	// io.CopyN(alldata, zpart, int64(f.UncompressedSize64))
	zpart.Read(alldata)
	if lenght == 0 {
		lenght = int64(f.UncompressedSize64)
	} else {
		lenght += offset
	}
	rdin = alldata[offset:lenght]
	defer zpart.Close()
	return rdin
}

func make_ROM(root *XMLNode, machine *MachineXML, cfg Mame2MRA, args Args) {
	if len(machine.Rom) == 0 {
		return
	}
	if args.Verbose {
		fmt.Println("Parsing ", machine.Name)
	}
	// Create nodes
	p := root.AddNode("rom").AddAttr("index", "0")
	zipname := machine.Name + ".zip"
	if len(machine.Cloneof) > 0 {
		zipname += "|" + machine.Cloneof + ".zip"
	}
	if len(cfg.Global.Zip.Alt) > 0 {
		zipname += "|" + cfg.Global.Zip.Alt
	}
	p.AddAttr("zip", zipname)
	p.AddAttr("md5", "None")
	if cfg.ROM.Ddr_load {
		p.AddAttr("address", "0x30000000")
	}
	regions := cfg.ROM.Order
	// Add regions unlisted in the config to the final list
	sorted_regs := make(map[string]bool)
	for _, r := range regions {
		sorted_regs[r] = true
	}
	cur_region := ""
	for _, rom := range machine.Rom {
		if cur_region != rom.Region {
			cur_region = rom.Region
			_, ok := sorted_regs[rom.Region]
			if !ok {
				regions = append(regions, cur_region)
			}
		}
	}
	var header *XMLNode
	if cfg.Header.Len > 0 {
		header = p.AddNode("part")
		header.indent_txt = true
	}
	pos := 0
	reg_offsets := make(map[string]int)

	var previous StartNode
	for _, reg := range regions {
		reg_cfg := find_region_cfg(machine, reg, cfg)
		if reg_cfg.Skip {
			continue
		}
		// Warn about unsorted regions
		_, sorted := sorted_regs[reg]
		if !sorted {
			fmt.Printf("\tunlisted region for sorting %s in %s\n", reg, machine.Name)
		}
		reg_roms := extract_region(reg_cfg, machine.Rom, cfg.ROM.Remove)
		// Do not skip empty regions, in case they have a minimum length to fill
		// Skip regions with "nodump" ROMs
		nodump := false
		for _, each := range reg_roms {
			if each.Status == "nodump" {
				nodump = true
			}
		}
		// Proceed with the ROM listing
		if delta := fill_upto(&pos, reg_cfg.start, p); delta < 0 {
			fmt.Printf(
				"\tstart offset overcome by 0x%X while parsing region %s in %s\n",
				-delta, reg, machine.Name)
		}
		sdram_bank_comment(p, pos, args.macros)
		// comment with start and length of region
		previous.add_length(pos)
		previous.node = p.AddNode(fmt.Sprintf("%s - starts at 0x%X", reg, pos))
		previous.node.comment = true
		previous.pos = pos
		start_pos := pos

		if nodump {
			if parse_custom(reg_cfg, p, machine, &pos, args) {
				fill_upto(&pos, start_pos+reg_cfg.Len, p)
			} else {
				p.AddNode(fmt.Sprintf("Skipping region %s because there is no dump known",
					reg_cfg.Name)).comment = true
			}
			continue
		}

		reg_offsets[reg] = pos
		reg_roms = apply_sort(reg_cfg, reg_roms, machine.Name)
		if reg_cfg.Singleton {
			// Singleton interleave case
			pos += parse_singleton(reg_roms, reg_cfg, p)
		} else {
			split, split_minlen := is_split(reg, machine, cfg)
			// Regular interleave case
			if (reg_cfg.Width != 0 && reg_cfg.Width != 8) && len(reg_roms) > 1 {
				parse_regular_interleave(split, split_minlen, reg, reg_roms, reg_cfg, p, machine, cfg, args, &pos)
			}
			if reg_cfg.Frac.Parts != 0 {
				pos += make_frac(p, reg_cfg, reg_roms)
			} else if reg_cfg.Width <= 8 || len(reg_roms) == 1 {
				parse_straight_dump(split, split_minlen, reg, reg_roms, reg_cfg, p, machine, cfg, &pos)
			}
		}
		fill_upto(&pos, start_pos+reg_cfg.Len, p)
	}
	previous.add_length(pos)
	make_devROM(p, machine, cfg, &pos)
	p.AddNode(fmt.Sprintf("Total 0x%X bytes - %d kBytes", pos, pos>>10)).comment = true
	make_patches(p, machine, cfg)
	if header != nil {
		make_header(header, reg_offsets, pos, cfg.Header, machine)
	}
}

func make_patches(root *XMLNode, machine *MachineXML, cfg Mame2MRA) {
	for _, each := range cfg.ROM.Patches {
		if is_family(each.Machine, machine) ||
			each.Setname == machine.Name ||
			(each.Machine == "" && each.Setname == "") {
			// apply the patch
			root.AddNode("patch", each.Value).AddAttr("offset", fmt.Sprintf("0x%X", each.Offset))
		}
	}
}

func set_header_offset(headbytes []byte, pos int, reverse bool, bits, offset int) {
	offset >>= bits
	headbytes[pos] = byte((offset >> 8) & 0xff)
	headbytes[pos+1] = byte(offset & 0xff)
	if reverse {
		aux := headbytes[pos]
		headbytes[pos] = headbytes[pos+1]
		headbytes[pos+1] = aux
	}
}

func rawdata2bytes(rawstr string) []byte {
	rawbytes := make([]byte, 0, 1024)
	datastr := strings.ReplaceAll(rawstr, "\n", " ")
	datastr = strings.ReplaceAll(datastr, "\t", " ")
	datastr = strings.TrimSpace(datastr)
	for _, hexbyte := range strings.Split(datastr, " ") {
		if hexbyte == "" {
			continue
		}
		conv, _ := strconv.ParseInt(hexbyte, 16, 0)
		rawbytes = append(rawbytes, byte(conv))
	}
	return rawbytes
}

func make_header(node *XMLNode, reg_offsets map[string]int,
	total int, cfg HeaderCfg, machine *MachineXML) {
	devs := machine.Devices
	headbytes := make([]byte, cfg.Len)
	for k := 0; k < cfg.Len; k++ {
		headbytes[k] = byte(cfg.Fill)
	}
	// Fill ROM offsets
	unknown_regions := make([]string, 0)
	if len(cfg.Offset.Regions) > 0 {
		pos := cfg.Offset.Start
		for _, r := range cfg.Offset.Regions {
			offset, ok := reg_offsets[r]
			if !ok {
				unknown_regions = append(unknown_regions, r)
				offset = 0
			}
			// fmt.Printf("region %s offset %X\n", r, offset)
			set_header_offset(headbytes, pos, cfg.Offset.Reverse, cfg.Offset.Bits, offset)
			pos += 2
		}
		//set_header_offset(headbytes, pos, cfg.Offset.Reverse, cfg.Offset.Bits, total)
	}
	if len(unknown_regions) > 0 {
		fmt.Printf("\tmissing region(s)")
		for _, uk := range unknown_regions {
			fmt.Printf(" %s", uk)
		}
		fmt.Printf(". Offset set to zero in the header (%s)\n", machine.Name)
	}
	// Manual headers
	for _, each := range cfg.Data {
		if (len(each.Machine) != 0 && !is_family(each.Machine, machine)) || (len(each.Setname) != 0 && each.Setname != machine.Name) {
			continue // skip it
		}
		pos := each.Pointer
		rawbytes := rawdata2bytes(each.Data)
		// if pos+len(rawbytes) > len(headbytes) {
		//  log.Fatal("Header pointer larger than declared header")
		// }
		copy(headbytes[pos:], rawbytes)
		pos += len(rawbytes)
	}
	// Device dependent values
	for _, d := range cfg.Dev {
		found := false
		for _, ref := range devs {
			if d.Dev == ref.Name {
				found = true
				break
			}
		}
		if found {
			if d.Byte >= len(headbytes) {
				log.Fatal("Header device-byte falls outside the header")
			}
			headbytes[d.Byte] = byte(d.Value)
		}
	}
	node.SetText(hexdump(headbytes, 8))
}

func make_frac(parent *XMLNode, reg_cfg *RegCfg, reg_roms []MameROM) int {
	dumped := 0
	if (len(reg_roms) % reg_cfg.Frac.Parts) != 0 {
		// There are not enough ROMs, so repeat the last one
		// This is useful in cases such as having 3bpp graphics
		missing := reg_cfg.Frac.Parts - (len(reg_roms) % reg_cfg.Frac.Parts)
		// filled contains the original ROM list with
		// fillers inserted at the end of each group of ROMs
		var filled []MameROM
		step := len(reg_roms) / missing
		for k := 0; k < missing; k++ {
			filled = append(filled, reg_roms[k*step:(k+1)*step]...)
			filled = append(filled, filled[len(filled)-1])
		}
		reg_roms = filled
		////fmt.Println("Added ", missing, " roms to the end")
		//for k, r := range reg_roms {
		//  fmt.Println(k, " - ", r.Name)
		//}
	}
	output_bytes := reg_cfg.Frac.Parts / reg_cfg.Frac.Bytes
	if (output_bytes % 2) != 0 {
		log.Fatal(fmt.Sprintf(
			"Region %s: frac output_bytes (%d) is not a multiple of 2",
			reg_cfg.Name, output_bytes))
	}
	// ROM entries
	var n *XMLNode
	group_size := 0
	group_start := 0
	frac_groups := len(reg_roms) / reg_cfg.Frac.Parts
	for k, r := range reg_roms {
		cnt := k / reg_cfg.Frac.Parts
		mod := k % reg_cfg.Frac.Parts
		if mod == 0 {
			if k != 0 && (reg_cfg.Rom_len != 0 || reg_cfg.Len != 0) {
				exp_size := reg_cfg.Rom_len * reg_cfg.Frac.Parts
				if reg_cfg.Len/frac_groups > exp_size {
					exp_size = reg_cfg.Len / frac_groups
				}
				fill_upto(&dumped, group_start+exp_size*cnt, parent)
			}
			n = parent.AddNode("interleave").AddIntAttr("output", output_bytes*8)
			group_size = 0
			group_start = dumped
		}
		m := n.AddNode("part").AddAttr("name", r.Name)
		if len(r.Crc) > 0 {
			m.AddAttr("crc", r.Crc)
		}
		m.AddAttr("map", make_frac_map(reg_cfg.Reverse, reg_cfg.Frac.Bytes,
			output_bytes, mod))
		dumped += r.Size
		group_size += r.Size
	}
	return dumped
}

func make_frac_map(reverse bool, bytes, total, step int) string {
	mapstr := make([]byte, total)
	for k := 0; k < total; k++ {
		mapstr[k] = '0'
	}
	c := byte('1')
	j := step * bytes
	js := 1
	if !reverse {
		j = total - j - 1
		js = -1
	}
	// fmt.Println("Reverse = ", reverse, "j = ", j, "total = ", total, " step = ", step)
	for i := 0; i < bytes; {
		mapstr[j] = c
		c = c + 1
		i++
		j += js
	}
	var builder strings.Builder
	builder.Grow(total)
	builder.Write(mapstr)
	return builder.String()
}

func extract_region(reg_cfg *RegCfg, roms []MameROM, remove []string) (ext []MameROM) {
	// Custom list
	if len(reg_cfg.Files) > 0 {
		// fmt.Println("Using custom files for ", reg_cfg.Name)
		ext = make([]MameROM, len(reg_cfg.Files))
		copy(ext, reg_cfg.Files)
		for k, _ := range ext {
			ext[k].Region = reg_cfg.Name
		}
		return
	}
	// MAME list
roms_loop:
	for _, r := range roms {
		if r.Region == reg_cfg.Name {
			for _, rm := range remove {
				if rm == r.Name {
					continue roms_loop
				}
			}
			ext = append(ext, r)
		}
	}
	return
}

func cmp_count(a, b string, rmext bool) bool {
	if rmext { // removes the file extension
		// this helps comparing file names like abc123.bin
		k := strings.LastIndex(a, ".")
		if k != -1 {
			a = a[0:k]
		}
		k = strings.LastIndex(b, ".")
		if k != -1 {
			b = b[0:k]
		}
	}
	re := regexp.MustCompile("[0-9]+")
	asub := re.FindAllString(a, -1)
	bsub := re.FindAllString(b, -1)
	kmax := len(asub)
	if len(bsub) < kmax {
		kmax = len(bsub)
	}
	low := true
	for k := 0; k < kmax; k++ {
		aint, _ := strconv.Atoi(asub[k])
		bint, _ := strconv.Atoi(bsub[k])
		if aint > bint {
			low = false
			break
		}
	}
	return low
}

func sort_byext(reg_cfg *RegCfg, roms []MameROM, setname string) {
	// If all the ROMs have the same extension,
	// it will sort by name instead
	allequal := true
	ext := ""
	for k, r := range roms {
		da := strings.LastIndex(r.Name, ".")
		if da == -1 {
			if ext != "" {
				allequal = false
				break
			} else {
				continue
			}
		} else {
			if k == 0 {
				ext = r.Name[da:]
				continue
			} else {
				if ext != r.Name[da:] {
					allequal = false
					break
				}
			}
		}
	}
	if !allequal {
		// Sort by extension
		sort.Slice(roms, func(i, j int) bool {
			var a *MameROM = &roms[i]
			var b *MameROM = &roms[j]
			da := strings.LastIndex(a.Name, ".")
			db := strings.LastIndex(b.Name, ".")
			if da == -1 {
				return true
			}
			if db == -1 {
				return false
			}
			if reg_cfg.Sort_alpha {
				return strings.Compare(a.Name[da:], b.Name[db:]) < 0
			} else {
				return cmp_count(a.Name[da:], b.Name[db:], false)
			}
		})
	} else {
		// All extensions were equal, so sort by name
		fmt.Printf("\tsorting %s by name as all extensions were equal (%s)\n", reg_cfg.Name, setname)
		sort.Slice(roms, func(i, j int) bool {
			var a *MameROM = &roms[i]
			var b *MameROM = &roms[j]
			if reg_cfg.Sort_alpha {
				return strings.Compare(a.Name, b.Name) < 0
			} else {
				return cmp_count(a.Name, b.Name, true)
			}
		})
	}
}

func sort_even_odd(reg_cfg *RegCfg, roms []MameROM, even_first bool) {
	if !even_first {
		log.Fatal("even_first==false not implemented")
	}
	if reg_cfg.Sort_reverse {
		log.Fatal("even_first==false not implemented")
	}
	base := make([]MameROM, len(roms))
	copy(base, roms)
	// Copy the even ones
	for i := 0; i < len(roms); i += 2 {
		roms[i>>1] = base[i]
	}
	half := len(roms) >> 1
	// Copy the odd ones
	for i := 1; i < len(roms); i += 2 {
		roms[(i>>1)+half] = base[i]
	}
}

func sort_ext_list(reg_cfg *RegCfg, roms []MameROM) {
	base := make([]MameROM, len(roms))
	copy(base, roms)
	k := 0
	for _, ext := range reg_cfg.Ext_sort {
		for i, _ := range base {
			if strings.HasSuffix(base[i].Name, ext) {
				roms[k] = base[i]
				k++
				break
			}
		}
	}
}

func sort_name_list(reg_cfg *RegCfg, roms []MameROM) {
	// fmt.Println("Applying name sorting ", reg_cfg.Name_sort)
	base := make([]MameROM, len(roms))
	copy(base, roms)
	k := 0
	for _, each := range reg_cfg.Name_sort {
		for i, _ := range base {
			if base[i].Name == each {
				roms[k] = base[i]
				k++
				break
			}
		}
	}
}

func sort_regex_list(reg_cfg *RegCfg, roms []MameROM) {
	// fmt.Println("Applying name sorting ", reg_cfg.Name_sort)
	base := make([]MameROM, len(roms))
	copy(base, roms)
	k := 0
	for _, each := range reg_cfg.Regex_sort {
		re := regexp.MustCompile(each)
		for i, _ := range base {
			if re.MatchString(base[i].Name) {
				roms[k] = base[i]
				k++
				break
			}
		}
	}
}

func sort_fullname(reg_cfg *RegCfg, roms []MameROM) {
	sort.Slice(roms, func(i, j int) bool {
		var a *MameROM = &roms[i]
		var b *MameROM = &roms[j]
		if reg_cfg.Sort_alpha {
			return strings.Compare(a.Name, b.Name) < 0
		} else {
			return cmp_count(a.Name, b.Name, true)
		}
	})
}

func apply_sequence(reg_cfg *RegCfg, roms []MameROM) []MameROM {
	kmax := len(roms)
	seqd := make([]MameROM, len(reg_cfg.Sequence))
	if len(roms) == 0 {
		fmt.Printf("Warning: attempting to sort empty region %s\n", reg_cfg.Name)
		return roms
	}
	copy(seqd, roms)
	for i, k := range reg_cfg.Sequence {
		if k >= kmax {
			k = 0 // Not necessarily an error, as some ROM sets may have more files than others
		}
		seqd[i] = roms[k]
	}
	return seqd
}

func apply_sort(reg_cfg *RegCfg, roms []MameROM, setname string) []MameROM {
	if len(reg_cfg.Sequence) > 0 {
		return apply_sequence(reg_cfg, roms)
	}
	if len(reg_cfg.Ext_sort) > 0 {
		sort_ext_list(reg_cfg, roms)
		return roms
	}
	if len(reg_cfg.Name_sort) > 0 {
		sort_name_list(reg_cfg, roms)
		return roms
	}
	if len(reg_cfg.Regex_sort) > 0 {
		sort_regex_list(reg_cfg, roms)
		return roms
	}
	if reg_cfg.Sort_even {
		sort_even_odd(reg_cfg, roms, true)
		return roms
	}
	if reg_cfg.Sort_byext {
		sort_byext(reg_cfg, roms, setname)
		if reg_cfg.Sort_reverse {
			base := make([]MameROM, len(roms))
			copy(base, roms)
			for i := 0; i < len(roms); i++ {
				roms[i] = base[len(roms)-1-i]
			}
		}
		return roms
	}
	if reg_cfg.Sort_alpha || reg_cfg.Sort {
		sort_fullname(reg_cfg, roms)
		return roms
	}
	return roms
}

func add_rom(parent *XMLNode, rom MameROM) *XMLNode {
	n := parent.AddNode("part").AddAttr("name", rom.Name)
	if len(rom.Crc) > 0 {
		n.AddAttr("crc", rom.Crc)
	}
	return n
}

func fill_upto(pos *int, fillto int, parent *XMLNode) int {
	if fillto == 0 {
		return 0
	}
	delta := fillto - *pos
	if delta <= 0 {
		return delta
	}
	parent.AddNode("part", " FF").AddAttr("repeat", fmt.Sprintf("0x%X", fillto-*pos))
	*pos += delta
	return delta
}

func find_region_cfg(machine *MachineXML, regname string, cfg Mame2MRA) *RegCfg {
	var best *RegCfg
	for k, each := range cfg.ROM.Regions {
		if each.Name == regname {
			if each.Setname == machine.Name {
				best = &cfg.ROM.Regions[k]
				break
			}
			if is_family(each.Machine, machine) || (each.Setname == "" && each.Machine == "" && best == nil) {
				best = &cfg.ROM.Regions[k]
			}
		}
	}
	// the region does not have a configuration in the TOML file, set a default one:
	if best == nil {
		best = &RegCfg{
			Name: regname,
		}
	}
	return best
}

func get_reverse(reg_cfg *RegCfg, name string) bool {
	for _, k := range reg_cfg.Overrules {
		for _, j := range k.Names {
			if j == name {
				return k.Reverse
			}
		}
	}
	return reg_cfg.Reverse
}

// if the region is marked for a blank at this point returns its length
// otherwise, zero
func is_blank(curpos int, reg string, machine *MachineXML, cfg Mame2MRA) (blank_len int) {
	blank_len = 0
	offset := 0
	for _, blank := range cfg.ROM.Blanks {
		if blank.Region != reg && blank.Region != "" {
			continue
		}
		if (blank.Machine == "" && blank.Setname == "") || // apply to all
			is_family(blank.Machine, machine) || // apply to machine
			(blank.Setname == machine.Name) { // apply to a setname
			offset = blank.Offset
			blank_len = blank.Len
		}
	}
	if offset != 0 && offset == curpos {
		return blank_len
	} else {
		return 0
	}
}

func parse_singleton(reg_roms []MameROM, reg_cfg *RegCfg, p *XMLNode) int {
	pos := 0
	if reg_cfg.Width != 16 && reg_cfg.Width != 32 {
		log.Fatal("jtframe mra: singleton only supported for width 16 and 32")
	}
	var n *XMLNode
	p.AddNode("Singleton region. The files are merged with themselves.").comment = true
	msb := (reg_cfg.Width / 8) - 1
	divider := reg_cfg.Width >> 3
	mapfmt := fmt.Sprintf("%%0%db", divider)
	for _, r := range reg_roms {
		n = p.AddNode("interleave").AddAttr("output", fmt.Sprintf("%d", reg_cfg.Width))
		mapbyte := 1
		if reg_cfg.Reverse {
			mapbyte = 1 << msb // 2 for 16 bits, 8 for 32 bits
		}
		for k := 0; k < divider; k++ {
			m := add_rom(n, r)
			m.AddAttr("offset", fmt.Sprintf("0x%04x", r.Size/divider*k))
			m.AddAttr("map", fmt.Sprintf(mapfmt, mapbyte))
			m.AddAttr("length", fmt.Sprintf("0x%04X", r.Size/divider))
			// Second half
			if reg_cfg.Reverse {
				mapbyte >>= 1
			} else {
				mapbyte <<= 1
			}
		}
		pos += r.Size
	}
	return pos
}

func parse_straight_dump(split, split_minlen int, reg string, reg_roms []MameROM, reg_cfg *RegCfg, p *XMLNode, machine *MachineXML, cfg Mame2MRA, pos *int) {
	reg_pos := 0
	start_pos := *pos
	for _, r := range reg_roms {
		offset := r.Offset
		if reg_cfg.No_offset {
			offset = 0
		} else {
			if delta := fill_upto(pos, ((offset&-2)-reg_pos)+*pos, p); delta < 0 {
				fmt.Printf("Warning: ROM start overcome at 0x%X (expected 0x%X - delta=%X)\n",
					*pos, ((offset&-2)-reg_pos)+*pos, delta)
				fmt.Printf("\t while parsing region %s (%s)\n", reg_cfg.Name, machine.Name)
			}
		}
		rom_pos := *pos
		// check if the next ROM should be split
		rom_len := 0
		var m *XMLNode
		if reg_cfg.Reverse {
			pp := p.AddNode("interleave").AddAttr("output", "16")
			m = add_rom(pp, r)
			m.AddAttr("map", "12")
		} else {
			m = add_rom(p, r)
		}
		// Parse ROM splits by marking the dumped ROM above
		// as only the first half, filling in a blank, and
		// adding the second half
		if *pos-start_pos <= split && *pos-start_pos+r.Size > split && split_minlen > (r.Size>>1) {
			fmt.Printf("\t-split on single ROM file at %X\n", split)
			rom_len = r.Size >> 1
			m.AddAttr("length", fmt.Sprintf("0x%X", rom_len))
			*pos += rom_len
			fill_upto(pos, *pos+split_minlen-rom_len, p)
			// second half
			if reg_cfg.Reverse {
				pp := p.AddNode("interleave").AddAttr("output", "16")
				m = add_rom(pp, r)
				m.AddAttr("map", "12")
			} else {
				m = add_rom(p, r)
			}
			m.AddAttr("length", fmt.Sprintf("0x%X", rom_len))
			m.AddAttr("offset", fmt.Sprintf("0x%X", rom_len))
			*pos += rom_len
		} else {
			if reg_cfg.Rom_len != 0 {
				m.AddAttr("length", fmt.Sprintf("0x%X", reg_cfg.Rom_len))
			}
			*pos += r.Size
		}
		if reg_cfg.Rom_len > r.Size {
			fill_upto(pos, reg_cfg.Rom_len+rom_pos, p)
		}
		reg_pos = *pos - start_pos
		if blank_len := is_blank(reg_pos, reg, machine, cfg); blank_len > 0 {
			fill_upto(pos, *pos+blank_len, p)
			p.AddNode(fmt.Sprintf("Blank ends at 0x%X", *pos)).comment = true
		}
		reg_pos = *pos - start_pos
	}
}

func parse_i8751(reg_cfg *RegCfg, p *XMLNode, machine *MachineXML, pos *int, args Args) bool {
	path := filepath.Join(args.firmware_dir, machine.Name+".s")
	f, e := os.Open(path)
	if e != nil {
		path := filepath.Join(args.firmware_dir, machine.Cloneof+".s")
		f, e = os.Open(path)
		if e != nil {
			log.Println("jtframe mra: cannot find custom firmware for ", machine.Name)
			return false
		}
	}
	f.Close()
	binname := fmt.Sprintf("mra%X%X.bin", rand.Int(), rand.Int())
	cmd := exec.Command("as31", "-Fbin", "-O"+binname, path)
	e = cmd.Run()
	if e != nil {
		log.Println("jtframe mra, problem running as31:\n\t", e)
		return false
	}
	// Read the result and add it as data
	bin, e := ioutil.ReadFile(binname)
	if e != nil {
		log.Println("jtframe mra, problem reading as31 output:\n\t", e)
		return false
	}
	*pos += len(bin)
	p.AddNode("Using custom firmware (no known dump)").comment = true
	node := p.AddNode("part")
	node.indent_txt = true
	node.SetText(hexdump(bin, 16))
	return true
}

func parse_custom(reg_cfg *RegCfg, p *XMLNode, machine *MachineXML, pos *int, args Args) bool {
	if reg_cfg.Custom.Dev == "" {
		return false
	}
	switch reg_cfg.Custom.Dev {
	case "i8751":
		{
			return parse_i8751(reg_cfg, p, machine, pos, args)
		}
	default:
		log.Fatal("jtframe mra: unsupported custom.dev=", reg_cfg.Custom.Dev)
	}
	return false
}

func parse_regular_interleave(split, split_minlen int, reg string, reg_roms []MameROM, reg_cfg *RegCfg, p *XMLNode, machine *MachineXML, cfg Mame2MRA, args Args, pos *int) {
	reg_pos := 0
	start_pos := *pos
	group_cnt := 0
	if args.Verbose {
		fmt.Printf("\tRegular interleave for %s (%s)\n", reg_cfg.Name, machine.Name)
	}
	if !reg_cfg.No_offset {
		// Try to determine from the offset the word-length of each ROM
		// as well as the isolated ones
		// fmt.Println("Parsing ", reg_cfg.Name)
		for k := 0; k < len(reg_roms); {
			// Try to make a group
			kmin := k
			kmax := kmin
			wlen := 8
			for j := kmin; j < len(reg_roms); j++ {
				if (reg_roms[kmin].Offset &^ 0xf) != (reg_roms[j].Offset &^ 0xf) {
					break
				}
				if reg_roms[j].Offset&1 != 0 {
					wlen = 1
				}
				if wlen > 1 && (reg_roms[j].Offset&2) != 0 {
					wlen = 2
				}
				if wlen > 2 && (reg_roms[j].Offset&4) != 0 {
					wlen = 4
				}
				kmax = j
			}
			if kmin != kmax {
				if args.Verbose {
					fmt.Printf("\tGroup found (%d-%d)\n", kmin, kmax)
				}
				group_cnt++
				if (kmax-kmin+1)*wlen != (reg_cfg.Width >> 3) {
					fmt.Printf("jtframe mra: the number of ROMs for the %d-bit region (%s) is not even (%s).\nUsing ROMs:\n",
						reg_cfg.Width, reg_cfg.Name, machine.Name)
					for j := kmin; j <= kmax; j++ {
						fmt.Printf("\t%s\n", reg_roms[j].Name)
					}
					os.Exit(1)
				}
			}
			for j := kmin; j <= kmax && kmin != kmax; j++ {
				reg_roms[j].group = group_cnt
				reg_roms[j].wlen = wlen
				if args.Verbose {
					fmt.Println("\t\t", reg_roms[j].Name)
				}
			}
			group_cnt += kmax - kmin + 1
			k = kmax + 1
		}
	} else {
		// If no_offset is set, then assume all are grouped together and the word length is 1 byte
		if (len(reg_roms) % (reg_cfg.Width / 8)) != 0 {
			log.Fatal(fmt.Sprintf("The number of ROMs for the %d-bit region (%s) is not even in %s",
				reg_cfg.Width, reg_cfg.Name, machine.Name))
		}
		for j, _ := range reg_roms {
			reg_roms[j].group = 1
			reg_roms[j].wlen = 1
		}
		group_cnt = len(reg_roms)
	}
	n := p
	deficit := 0
	for split_phase := 0; split_phase <= split && split_phase < 2; split_phase++ {
		if split_phase == 1 {
			p.AddNode(fmt.Sprintf("ROM split at %X (%X)", *pos, *pos-start_pos)).comment = true
		}
		chunk0 := *pos
		for k := 0; k < len(reg_roms); {
			r := reg_roms[k]
			mapstr := ""
			rom_cnt := 1
			if r.group != 0 {
				// make interleave node at the expected position
				if deficit > 0 {
					fill_upto(pos, *pos+deficit, p)
				}
				reg_pos = *pos - start_pos
				offset := r.Offset
				if reg_cfg.No_offset {
					offset = 0
				}
				fill_upto(pos, ((offset&-2)-reg_pos)+*pos, p)
				deficit = 0
				n = p.AddNode("interleave").AddAttr("output", fmt.Sprintf("%d", reg_cfg.Width))
				if args.Verbose {
					fmt.Printf("Made %d-bit interleave for %s\n", reg_cfg.Width, reg_cfg.Name)
				}
				// Prepare the map
				for j := r.wlen; j > 0; j-- {
					mapstr = mapstr + strconv.Itoa(j)
				}
				for j := r.wlen; j < (reg_cfg.Width >> 3); j++ {
					mapstr = "0" + mapstr
				}
				rom_cnt = (reg_cfg.Width >> 3) / r.wlen
			}
			process_rom := func(j int) {
				r = reg_roms[j]
				if args.Verbose {
					fmt.Printf("Parsing %s (%d-byte words - mapstr=%s)\n", r.Name, r.wlen, mapstr)
				}
				m := add_rom(n, r)
				if mapstr != "" {
					m.AddAttr("map", mapstr)
					mapstr = mapstr[r.wlen:] + mapstr[0:r.wlen] // rotate the active byte
				}
				if split != 0 {
					m.AddAttr("length", fmt.Sprintf("0x%X", r.Size/2))
					if split_phase == 1 {
						m.AddAttr("offset", fmt.Sprintf("0x%X", r.Size/2))
					}
					*pos += r.Size / 2
				} else {
					*pos += r.Size
					if reg_cfg.Rom_len > r.Size {
						deficit += reg_cfg.Rom_len - r.Size
					}
				}
				reg_pos = *pos - start_pos
				if blank_len := is_blank(reg_pos, reg, machine, cfg); blank_len > 0 {
					fill_upto(pos, *pos+blank_len, p)
					p.AddNode(fmt.Sprintf("Blank ends at 0x%X", *pos)).comment = true
				}
			}
			if reg_cfg.Reverse {
				for j := k + rom_cnt - 1; j >= k; j-- {
					if reg_roms[j].group == 0 && get_reverse(reg_cfg, reg_roms[j].Name) {
						mapstr = "12" // Should this try to contemplate other cases different from 16 bits?
						n = p.AddNode("interleave").AddAttr("output", "16")
					}
					process_rom(j)
				}
			} else {
				for j := k; j < k+rom_cnt; j++ {
					process_rom(j)
				}
			}
			n = p
			k += rom_cnt
		}
		if *pos-chunk0 < split_minlen {
			// fmt.Printf("\tsplit minlen = %x (dumped = %X) \n", split_minlen, *pos-chunk0)
			fill_upto(pos, split_minlen+chunk0, p)
		}
	}
}
