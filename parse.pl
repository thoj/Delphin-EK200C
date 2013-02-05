use strict;
use warnings;

my $header_size = 24;
my $header;
my $buf;
my $data;
my $i = 0;
my $lines = 1;
while ( read(*STDIN, $header, $header_size) ) {
	$i ++;
	my ($version, $code, $length, $ukn, $sequence) = unpack("nnNNN", $header);
	if ( $length > 0 ) {
		read(*STDIN, $data, $length);
		next if not $code == 128;
		my (@t) = unpack("(VV)*", $data);
		for (my $i = 0; $i < scalar @t; $i+=2) {#			print $x;
			my $value = ($t[$i+1] >> 0) & (1 << 26) - 1;
			my $chan = ($t[$i+1] >> 27) & (1 << 5) -1;
			printf("%2d %9d %9d %s\n", $chan, $t[$i], $value, unpack("b32", pack("V", $t[$i+1])));
			$lines ++;
		}
	}
}


