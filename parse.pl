use strict;
use warnings;

my $header_size = 24;
my $header;
my $buf;
my $data;
my $i = 0;
my $lines = 1;

my $range_max = 10000; #mV
my $range_min = -10000; #mV

while ( read(*STDIN, $header, $header_size) ) {
	$i ++;
	my ($version, $code, $length, $ukn, $sequence) = unpack("nnNNN", $header);
	if ( $length > 0 ) {
		read(*STDIN, $data, $length);
		next if not $code == 128;
		my (@t) = unpack("(VV)*", $data);
		for (my $i = 0; $i < scalar @t; $i+=2) {
			my $value = (($t[$i+1] >> 0) & (1 << 26) - 1);
			if ($value > (2**25)) {
				$value = -(($value - (2**26))/ 2**24) * ($range_min*1.02);
			} else {
				$value = ($value / 2**24) * ($range_max * 1.0343);
			}
			my $chan = ($t[$i+1] >> 27) & (1 << 6) -1;
			printf("%2d %9d %8.2f\n", $chan, $t[$i], $value);
			$lines ++;
		}
	}
}


