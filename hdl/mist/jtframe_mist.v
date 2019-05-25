/*  This file is part of JT_GNG.
    JT_GNG program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    JT_GNG program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with JT_GNG.  If not, see <http://www.gnu.org/licenses/>.

    Author: Jose Tejada Gomez. Twitter: @topapate
    Version: 1.0
    Date: 7-3-2019 */

`timescale 1ns/1ps

module jtframe_mist(
    input           clk_sys,
    input           clk_rom,
    input           clk_vga,
    input           pxl_cen,
    input           pll_locked,
    // interface with microcontroller
    output  [31:0]  status,
    // Base video
    input   [1:0]   osd_rotate,
    input   [3:0]   game_r,
    input   [3:0]   game_g,
    input   [3:0]   game_b,
    input           LHBL,
    input           LVBL,
    input           hs,
    input           vs,
    // VGA-compatible video
    input   [5:0]   vga_r,
    input   [5:0]   vga_g,
    input   [5:0]   vga_b,
    input           vga_hsync,
    input           vga_vsync,
    // MiST VGA pins
    output  [5:0]   VGA_R,
    output  [5:0]   VGA_G,
    output  [5:0]   VGA_B,
    output          VGA_HS,
    output          VGA_VS,
    // SDRAM interface
    inout  [15:0]   SDRAM_DQ,       // SDRAM Data bus 16 Bits
    output [12:0]   SDRAM_A,        // SDRAM Address bus 13 Bits
    output          SDRAM_DQML,     // SDRAM Low-byte Data Mask
    output          SDRAM_DQMH,     // SDRAM High-byte Data Mask
    output          SDRAM_nWE,      // SDRAM Write Enable
    output          SDRAM_nCAS,     // SDRAM Column Address Strobe
    output          SDRAM_nRAS,     // SDRAM Row Address Strobe
    output          SDRAM_nCS,      // SDRAM Chip Select
    output [1:0]    SDRAM_BA,       // SDRAM Bank Address
    input           SDRAM_CLK,      // SDRAM Clock
    output          SDRAM_CKE,      // SDRAM Clock Enable
    // ROM access from game
    input           sdram_req,
    output          sdram_ack,
    input  [21:0]   sdram_addr,
    output [31:0]   data_read,
    output          data_rdy,
    output          loop_rst,
    input           refresh_en,
    // SPI interface to arm io controller
    output          SPI_DO,
    input           SPI_DI,
    input           SPI_SCK,
    input           SPI_SS2,
    input           SPI_SS3,
    input           SPI_SS4,
    input           CONF_DATA0,
    // ROM load from SPI
    output [21:0]   ioctl_addr,
    output [ 7:0]   ioctl_data,
    output          ioctl_wr,
    input  [21:0]   prog_addr,
    input  [ 7:0]   prog_data,
    input  [ 1:0]   prog_mask,
    input           prog_we,
    output          downloading,
//////////// board
    output            rst,      // synchronous reset
    output            rst_n,    // asynchronous reset
    output            game_rst,
    // reset forcing signals:
    input             dip_flip, // A change in dip_flip implies a reset
    input             rst_req,
    // Sound
    input   [15:0]    snd,
    output            AUDIO_L,
    output            AUDIO_R,
    // joystick
    output     [9:0]  game_joystick1,
    output     [9:0]  game_joystick2,
    output     [1:0]  game_coin,
    output     [1:0]  game_start,
    output            game_pause,
    output            game_service,
    // Debug
    output            LED,
    output     [3:0]  gfx_en
);

parameter SIGNED_SND=1'b0;
parameter THREE_BUTTONS=1'b0;
parameter GAME_INPUTS_ACTIVE_HIGH=1'b0;
parameter CONF_STR = "";
parameter CONF_STR_LEN = 0;

// control
wire [31:0]   joystick1, joystick2;
wire          ps2_kbd_clk, ps2_kbd_data;
wire          osd_shown;

assign AUDIO_R = AUDIO_L;

///////////////// LED is on while
// downloading, PLL lock lost, OSD is shown or in reset state
assign LED = ~( downloading | ~pll_locked | osd_shown | rst );

jtgng_mist_base #(.CONF_STR(CONF_STR), .CONF_STR_LEN(CONF_STR_LEN),
    .SIGNED_SND(SIGNED_SND)
) u_base(
    .rst            ( rst           ),
    .clk_sys        ( clk_sys       ),
    .clk_vga        ( clk_vga       ),
    .clk_rom        ( clk_rom       ),
    .pxl_cen        ( pxl_cen       ),
    .SDRAM_CLK      ( SDRAM_CLK     ),
    .osd_shown      ( osd_shown     ),
    // Base video
    .osd_rotate     ( osd_rotate    ),
    .game_r         ( game_r        ),
    .game_g         ( game_g        ),
    .game_b         ( game_b        ),
    .LHBL           ( LHBL          ),
    .LVBL           ( LVBL          ),
    .hs             ( hs            ),
    .vs             ( vs            ),
    // VGA video (without OSD)
    .vga_r          ( vga_r         ),
    .vga_g          ( vga_g         ),
    .vga_b          ( vga_b         ),
    .vga_hsync      ( vga_hsync     ),
    .vga_vsync      ( vga_vsync     ),  
    // MiST VGA pins (includes OSD)
    .VIDEO_R        ( VGA_R         ),
    .VIDEO_G        ( VGA_G         ),
    .VIDEO_B        ( VGA_B         ),
    .VIDEO_HS       ( VGA_HS        ),
    .VIDEO_VS       ( VGA_VS        ),
    // SPI interface to arm io controller
    .SPI_DO         ( SPI_DO        ),
    .SPI_DI         ( SPI_DI        ),
    .SPI_SCK        ( SPI_SCK       ),
    .SPI_SS2        ( SPI_SS2       ),
    .SPI_SS3        ( SPI_SS3       ),
    .SPI_SS4        ( SPI_SS4       ),
    .CONF_DATA0     ( CONF_DATA0    ),
    // control
    .status         ( status        ),
    .joystick1      ( joystick1     ),
    .joystick2      ( joystick2     ),
    .ps2_kbd_clk    ( ps2_kbd_clk   ),
    .ps2_kbd_data   ( ps2_kbd_data  ),
    // audio
    .clk_dac        ( clk_sys       ),
    .snd            ( snd           ),
    .snd_pwm        ( AUDIO_L       ),
    // ROM load from SPI
    .ioctl_addr     ( ioctl_addr    ),
    .ioctl_data     ( ioctl_data    ),
    .ioctl_wr       ( ioctl_wr      ),
    .downloading    ( downloading   )
);

jtgng_board #(.THREE_BUTTONS(THREE_BUTTONS),
    .GAME_INPUTS_ACTIVE_HIGH(GAME_INPUTS_ACTIVE_HIGH)
) u_board(
    .rst            ( rst             ),
    .rst_n          ( rst_n           ),
    .game_rst       ( game_rst        ),
    .dip_flip       ( dip_flip        ),
    .rst_req        ( rst_req         ),
    .downloading    ( downloading     ),

    .clk_sys        ( clk_sys         ),
    .clk_rom        ( clk_rom         ),
    // joystick
    .ps2_kbd_clk    ( ps2_kbd_clk     ),
    .ps2_kbd_data   ( ps2_kbd_data    ),
    .board_joystick1( joystick1[9:0]  ),
    .board_joystick2( joystick2[9:0]  ),
`ifndef SIM_INPUTS
    .game_joystick1 ( game_joystick1  ),
    .game_joystick2 ( game_joystick2  ),
    .game_coin      ( game_coin       ),
    .game_start     ( game_start      ),
`endif
    .game_pause     ( game_pause      ),
    .game_service   ( game_service    ),
    // SDRAM interface
    .SDRAM_DQ       ( SDRAM_DQ        ),
    .SDRAM_A        ( SDRAM_A         ),
    .SDRAM_DQML     ( SDRAM_DQML      ),
    .SDRAM_DQMH     ( SDRAM_DQMH      ),
    .SDRAM_nWE      ( SDRAM_nWE       ),
    .SDRAM_nCAS     ( SDRAM_nCAS      ),
    .SDRAM_nRAS     ( SDRAM_nRAS      ),
    .SDRAM_nCS      ( SDRAM_nCS       ),
    .SDRAM_BA       ( SDRAM_BA        ),
    .SDRAM_CKE      ( SDRAM_CKE       ),
    // SDRAM controller
    .loop_rst       ( loop_rst        ),
    .sdram_addr     ( sdram_addr      ),
    .sdram_req      ( sdram_req       ),
    .sdram_ack      ( sdram_ack       ),
    .data_read      ( data_read       ),
    .data_rdy       ( data_rdy        ),
    .refresh_en     ( refresh_en      ),
    .prog_addr      ( prog_addr       ),
    .prog_data      ( prog_data       ),
    .prog_mask      ( prog_mask       ),
    .prog_we        ( prog_we         ),
    // Debug
    .gfx_en         ( gfx_en          )
);

endmodule // jtframe