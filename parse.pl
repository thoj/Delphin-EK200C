use strict;
use warnings;

my $header_size = 24;
my $header;
my $buf;
my $data  = "";
my $i     = 0;
my $lines = 1;

my $range_max = 10000;     #mV
my $range_min = -10000;    #mV

my $adj = {
    4 => { #default
        0 => 6.2056789,
        1 => 1.0003873,
        2 => 3.5046681 * 10**-9,
        3 => -2.0937999 * 10**-13
    },
    5 => { #default
        0 => 6.2053809,
        1 => 1.000379,
        2 => 3.0443865 * 10**-9,
        3 => -1.6165593 * 10**-13
    },
    -1 => { #default
        0 => 6.2053809,
        1 => 1.000379,
        2 => 3.0443865 * 10**-9,
        3 => -1.6165593 * 10**-13
    }
};

#8131862 = 9994

# the max raw value changes depending on update rate!
#my $raw_max = 8388608; # 23 bit max (?!)
#my $raw_max = 8385944; # Max raw value at overvoltage (about 10.3V)
#my $raw_max = 8144000;  # Seems correct if adjusted. Within 0.1mV of DataService
#my $raw_max = 4194304 ; # 22 bit max (50Hz)
my $raw_max  = 2**22/1.03 ; # 22 bit max (50Hz) (Close < 0.1mV) Ciel(2**22/1.03)

while ( read( *STDIN, $header, $header_size ) ) {
    $i++;

    #Read header. Constant size.
    my ( $version, $code, $length, $ukn, $sequence, $data_cs, $header_cs ) =
      unpack( "nnNNNNN", $header );

    if ( $length > 0 ) {
        if ( read( *STDIN, $data, $length ) != $length ) {
            print "Short read!\n";
        }
    }
    else {
        $data = "";
    }

    #Channel Data
    if ( $code == 128 ) {
        $lines = 1;
        my (@t) = unpack( "(VV)*", $data );
        for ( my $i = 0 ; $i < scalar @t ; $i += 2 ) {
            my $raw_value = ( ( $t[ $i + 1 ] >> 0 ) & ( 1 << 26 ) - 1 );
            my $value = 0;
            if ( $raw_value > ( 2**25 ) ) {
                $value = -( ( $raw_value - ( 2**26 ) ) / $raw_max ) * ($range_min);
            }
            else {
                $value = ( $raw_value / $raw_max ) * ($range_max);
            }
            my $chan = ( $t[ $i + 1 ] >> 27 ) & ( 1 << 6 ) - 1;
            if ( defined $adj->{$chan} ) {
                $value =
                  $adj->{$chan}{0} +
                  $adj->{$chan}{1} * $value +
                  $adj->{$chan}{2} * $value**2 +
                  $adj->{$chan}{3} * $value**3;
            } else {
                $value =
                  $adj->{-1}{0} +
                  $adj->{-1}{1} * $value +
                  $adj->{-1}{2} * $value**2 +
                  $adj->{-1}{3} * $value**3;
		}
            printf( "%2d %9d %12.2f %10d %032B\n", $chan, $t[$i], $value, $raw_value, $t[$i+1]);
            $lines++;
        }
        print "\n" if $lines > 1;
    }

    # Some value?
    #elsif ($code == 130) {
    #	my $value = unpack("V", $data);
    #	print "Junction Temperature: ".unpack("b*", $data)."\n";
    #}
    #Dump Unknown Packets
    else {
        printf( "UP V:%d C:%5d L:%2d U:%3d S:%7d HC:%8x DC:%8x> ",
            $version, $code, $length, $ukn, $sequence, $header_cs, $data_cs );
        if ( $length > 0 ) {
            print unpack( "H* ", $data );
        }
        print "\n";
    }
}
