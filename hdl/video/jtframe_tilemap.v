/*  This file is part of JTFRAME.
    JTFRAME program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    JTFRAME program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with JTFRAME.  If not, see <http://www.gnu.org/licenses/>.

    Author: Jose Tejada Gomez. Twitter: @topapate
    Version: 1.0
    Date: 15-12-2022 */
    

// Generic tile map generator with no scroll
// The ROM data must be in these format
// code, H parts, V part
// pixel data is 4bpp, and arrives in four bytes. Each byte is for a plane

module jtframe_tilemap #( parameter
    SIZE =  8,    // 8x8, 16x16 or 32x32
    VA   = 10,
    CW   = 12,
    PW   =  8,
    VR   = SIZE==8 ? CW+3 : SIZE==16 ? CW+5 : CW+7,
    MAP_HW = 8,    // size of the map in pixels
    MAP_VW = 8,
    FLIP_MSB  = 1, // set to 0 for scroll tile maps
    XOR_HFLIP = 0, // set to 1 so hflip gets ^ with flip
    XOR_VFLIP = 0  // set to 1 so vflip gets ^ with flip
)(
    input              rst,
    input              clk,
    input              pxl_cen,

    input        [8:0] vdump,
    input        [8:0] hdump,
    input              blankn,  // if !blankn there are no ROM requests
    input              flip,    // Screen flip

    output    [VA-1:0] vram_addr,

    input     [CW-1:0] code,
    input     [PW-5:0] pal,
    input              hflip,
    input              vflip,

    output reg [VR-1:0]rom_addr,
    input      [31:0]  rom_data,
    output reg         rom_cs,
    input              rom_ok,      // ignored. It assumes that data is always right

    output     [PW-1:0]pxl
);

localparam VW = SIZE==8 ? 3 : SIZE==16 ? 4:5;

reg  [  31:0] pxl_data;
reg  [PW-5:0] cur_pal, nx_pal;
wire          vflip_g;
reg           hflip_g, nx_hf;
reg     [8:0] heff;
wire    [8:0] veff;

// not flipping the MSB is usually needed in scroll layers
assign veff = vdump ^ { FLIP_MSB[0]&flip, {MAP_VW-1{flip}}};

always @* begin
    heff = hdump ^ {MAP_HW{flip}};
    if( flip ) heff = heff - 9'd7;
end

initial begin
    if( SIZE==32 ) begin
        $display("WARNING %m: SIZE=32 has not been tested");
    end
end

assign pxl       = { cur_pal, hflip_g ? {pxl_data[24], pxl_data[16], pxl_data[8], pxl_data[0]} :
                                        {pxl_data[31], pxl_data[23], pxl_data[15], pxl_data[7]} };
assign vflip_g   = (flip & XOR_VFLIP[0])^vflip;

assign vram_addr[VA-1-:MAP_VW-VW]=veff[MAP_VW-1:VW];
assign vram_addr[0+:MAP_HW-VW] = heff[MAP_HW-1:VW];

always @(posedge clk, posedge rst) begin
    if( rst ) begin
        rom_cs   <= 0;
        rom_addr <= 0;
        pxl_data <= 0;
        cur_pal  <= 0;
        hflip_g  <= 0;
    end else if(pxl_cen) begin
        if( heff[2:0]==0 ) begin
            rom_cs <= ~rst & blankn;
            rom_addr[0+:VW] <= veff[0+:VW]^{VW{vflip_g}};
            rom_addr[VR-1-:CW] <= code;
            if( SIZE==16 ) rom_addr[VW]      <= heff[3];
            if( SIZE==32 ) rom_addr[VW+1-:2] <= heff[4:3];
            pxl_data <= rom_data;
            // draw information is eight pixels behind
            nx_pal   <= pal;
            cur_pal  <= nx_pal;
            nx_hf    <= (flip & XOR_HFLIP[0])^hflip;
            hflip_g  <= nx_hf;
        end else begin
            pxl_data <= hflip_g ? (pxl_data>>1) : (pxl_data<<1);
        end
    end
end

endmodule