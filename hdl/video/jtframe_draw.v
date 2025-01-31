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
    Date: 18-12-2022 */

// Draws one line of a 16x16 tile
// It could be extended to 32x32 easily

module jtframe_draw#( parameter
    CW    = 12,    // code width
    PW    =  8,    // pixel width (lower four bits come from ROM)
    ZW    =  6,    // zoom step width
    SWAPH =  0     // swaps the two horizontal halves of the tile
)(
    input               rst,
    input               clk,

    input               draw,
    output reg          busy,
    input    [CW-1:0]   code,
    input      [ 8:0]   xpos,
    input      [ 3:0]   ysub,

    // optional zoom, keep at zero for no zoom
    input    [ZW-1:0]   hzoom,
    input               hz_keep, // set to 0 on the first tile of a multi-tile
                                 // sprite, 1 for the rest of the tiles
    input               hflip,
    input               vflip,
    input      [PW-5:0] pal,

    output     [CW+6:2] rom_addr,
    output reg          rom_cs,
    input               rom_ok,
    input      [31:0]   rom_data,

    output reg [ 8:0]   buf_addr,
    output              buf_we,
    output     [PW-1:0] buf_din
);

// Each tile is 16x16 and comes from the same ROM
// but it looks like the sprites have the two 8x16 halves swapped

reg    [31:0] pxl_data;
reg           rom_lsb;
reg    [ 3:0] cnt;
wire   [ 3:0] ysubf, pxl;
reg  [ZW-1:0] hz_cnt;
wire [ZW-1:0] nx_hz;
wire          skip;

assign ysubf   = ysub^{4{vflip}};
assign buf_din = { pal, pxl };
assign pxl     = hflip ?
    { pxl_data[31], pxl_data[23], pxl_data[15], pxl_data[ 7] } :
    { pxl_data[24], pxl_data[16], pxl_data[ 8], pxl_data[ 0] };

assign rom_addr = { code, rom_lsb^SWAPH[0], ysubf[3:0] };
assign buf_we   = busy & ~cnt[3];
assign { skip, nx_hz } = {1'b0, hz_cnt}+{1'b0,hzoom};

always @(posedge clk, posedge rst) begin
    if( rst ) begin
        rom_cs   <= 0;
        buf_addr <= 0;
        pxl_data <= 0;
        busy     <= 0;
        cnt      <= 0;
        hz_cnt   <= 0;
    end else begin
        if( !busy ) begin
            if( draw ) begin
                rom_lsb <= hflip; // 14+4 = 18 (+2=20)
                rom_cs  <= 1;
                busy    <= 1;
                cnt     <= 8;
                if( !hz_keep ) begin
                    hz_cnt   <= 0;
                    buf_addr <= xpos;
                end
            end
        end else begin
            if( rom_ok && rom_cs && cnt[3]) begin
                pxl_data <= rom_data;
                cnt[3]   <= 0;
                if( rom_lsb^hflip ) begin
                    rom_cs <= 0;
                end else begin
                    rom_cs <= 1;
                end
            end
            if( !cnt[3] ) begin
                hz_cnt   <= nx_hz;
                cnt      <= cnt+1'd1;
                pxl_data <= hflip ? pxl_data << 1 : pxl_data >> 1;
                rom_lsb  <= ~hflip;
                if( !skip ) buf_addr <= buf_addr+1'd1;
                if( cnt[2:0]==7 && !rom_cs ) busy <= 0;
            end
        end
    end
end

endmodule