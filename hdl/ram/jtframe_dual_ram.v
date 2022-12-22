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
    Date: 27-10-2017 */

// Generic dual port RAM with clock enable
// parameters:
//      dw      => Data bit width, 8 for byte-based memories
//      aw      => Address bit width, 10 for 1kB
//      simfile => binary file to load during simulation
//      simhexfile => hexadecimal file to load during simulation
//      synfile => hexadecimal file to load for synthesis

/* verilator lint_off MULTIDRIVEN */

module jtframe_dual_ram #(parameter dw=8, aw=10,
    simfile="", simhexfile="",
    synfile="",
    ascii_bin=0,  // set to 1 to read the ASCII file as binary
    dumpfile="dump.hex"
)(
    // Port 0
    input   clk0,
    input   [dw-1:0] data0,
    input   [aw-1:0] addr0,
    input   we0,
    output  [dw-1:0] q0,
    // Port 1
    input   clk1,
    input   [dw-1:0] data1,
    input   [aw-1:0] addr1,
    input   we1,
    output  [dw-1:0] q1
    `ifdef JTFRAME_DUAL_RAM_DUMP
    ,input dump
    `endif
);

    jtframe_dual_ram_cen #(
        .dw         ( dw        ),
        .aw         ( aw        ),
        .simfile    ( simfile   ),
        .simhexfile ( simhexfile),
        .synfile    ( synfile   ),
        .ascii_bin  ( ascii_bin ),
        .dumpfile   ( dumpfile  )
    ) u_ram (
        .clk0   ( clk0  ),
        .cen0   ( 1'b1  ),
        .clk1   ( clk1  ),
        .cen1   ( 1'b1  ),
        // Port 0
        .data0  ( data0 ),
        .addr0  ( addr0 ),
        .we0    ( we0   ),
        .q0     ( q0    ),
        // Port 1
        .data1  ( data1 ),
        .addr1  ( addr1 ),
        .we1    ( we1   ),
        .q1     ( q1    )
        `ifdef JTFRAME_DUAL_RAM_DUMP
        ,.dump  ( dump  )
        `endif
    );
endmodule



module jtframe_dual_ram_cen #(parameter dw=8, aw=10,
    simfile="", simhexfile="",
    synfile="",
    ascii_bin=0,  // set to 1 to read the ASCII file as binary
    dumpfile="dump" // do not add an extension to the name
)(
    input   clk0,
    input   cen0,
    input   clk1,
    input   cen1,
    // Port 0
    input   [dw-1:0] data0,
    input   [aw-1:0] addr0,
    input   we0,
    output [dw-1:0] q0,
    // Port 1
    input   [dw-1:0] data1,
    input   [aw-1:0] addr1,
    input   we1,
    output [dw-1:0] q1
    `ifdef JTFRAME_DUAL_RAM_DUMP
    ,input dump
    `endif
);

`ifdef SIMULATION
localparam SIMULATION=1;
`else
localparam SIMULATION=0;
`endif

`ifdef POCKET
localparam POCKET=1;
`else
localparam POCKET=0;
`endif

generate
    if( !SIMULATION && POCKET && aw<13 && dw<=8 ) begin
            jtframe_pocket_dualram u_pocket_ram(
                .address_a( addr0   ),
                .address_b( addr1   ),
                .clock_a  ( clk0    ),
                .clock_b  ( clk1    ),
                .data_a   ( data0   ),
                .data_b   ( data1   ),
                .enable_a ( cen0    ),
                .enable_b ( cen1    ),
                .wren_a   ( we0     ),
                .wren_b   ( we1     ),
                .q_a      ( q0      ),
                .q_b      ( q1      )
            );
        end else begin
            reg [dw-1:0] qq0, qq1;
            (* ramstyle = "no_rw_check" *) reg [dw-1:0] mem[0:(2**aw)-1];

            assign { q0, q1 } = { qq0, qq1 };

            always @(posedge clk0) if(cen0) begin
                qq0 <= mem[addr0];
                if(we0) mem[addr0] <= data0;
            end

            always @(posedge clk1) if(cen1) begin
                qq1 <= mem[addr1];
                if(we1) mem[addr1] <= data1;
            end

            /* verilator lint_off WIDTH */
            `ifdef SIMULATION
                // Content dump for simulation debugging
                `ifdef JTFRAME_DUAL_RAM_DUMP
                    integer fdump=0, dumpcnt, fcnt=32'h30303030;
                    reg dumpl;



                    always @(posedge clk1) begin
                        dumpl <= dump;
                        if( dump & ~dumpl ) begin
                            $display("INFO: %m contents dumped to %s", dumpfile );
                            fdump=$fopen( {dumpfile, "_", fcnt[23:0],".hex"},"w");
                            for( dumpcnt=0; dumpcnt<2**aw; dumpcnt=dumpcnt+1 )
                                $fdisplay(fdump,"%0X", mem[dumpcnt]);
                            $fclose(fdump);
                            // increment fcnt
                            fcnt[7:0] <= fcnt[7:0]==8'h39 ? 8'h30 : fcnt[7:0]+1'd1;
                            if( fcnt[7:0]==8'h39 ) begin
                                fcnt[15-:8] <= fcnt[15-:8]==8'h39 ? 8'h30 : fcnt[15-:8]+1'd1;
                                if( fcnt[15-:8]==8'h39 ) begin
                                    fcnt[23-:8] <= fcnt[23-:8]==8'h39 ? 8'h30 : fcnt[23-:8]+1'd1;
                                end
                            end
                        end
                    end
                `endif

                integer f, readcnt;
                initial begin
                    for( f=0; f<(2**aw)-1;f=f+1) begin
                        mem[f] = 0;
                    end
                    if( simfile != "" ) begin
                        f=$fopen(simfile,"rb");
                        if( f != 0 ) begin
                            readcnt=$fread( mem, f );
                            $display("INFO: Read %14s (%4d bytes/%2d%%) for %m",
                                simfile, readcnt, readcnt*100/(2**aw));
                            if( readcnt != 2**aw )
                                $display("      the memory was not filled by the file data");
                            $fclose(f);
                        end else begin
                            $display("WARNING: %m cannot open file: %s", simfile);
                        end
                        end
                    else begin
                        if( simhexfile != "" ) begin
                            $readmemh(simhexfile,mem);
                            $display("INFO: Read %14s (hex) for %m", simhexfile);
                        end else begin
                            if( synfile!= "" ) begin
                                if( ascii_bin==1 )
                                    $readmemb(synfile,mem);
                                else
                                    $readmemh(synfile,mem);
                                $display("INFO: Read %14s (hex) for %m", synfile);
                            end else
                                for( readcnt=0; readcnt<2**aw; readcnt=readcnt+1 )
                                    mem[readcnt] = {dw{1'b0}};
                        end
                    end
                end
            `else
                // file for synthesis:
                initial if(synfile!="" ) begin
                    if( ascii_bin==1 )
                        $readmemb(synfile,mem);
                    else
                        $readmemh(synfile,mem);
                end
            `endif
        end
endgenerate

/* verilator lint_on WIDTH */
endmodule
/* verilator lint_on MULTIDRIVEN */