use strict;
use warnings;

my $header_size = 24;
my $header;
my $buf;
my $data;
my $i = 0;

while ( read(*STDIN, $header, $header_size) ) {
	$i ++;
	my ($version, $code, $length, $ukn, $sequence) = unpack("nnNNN", $header);
	if ( $length > 0 ) {
		read(*STDIN, $data, $length);
		next if not $code == 128;
#		my @f = unpack("(b18)*", $data);
		my @f = unpack("(h4)*", $data);
#		printf("%5d %2d %5d (%2d): ", $code, $length, $sequence, scalar @f);
		foreach my $x (@f) {
			print $x;
		#	printf("%20s", $x)
#			printf("%10s ", unpack("h*",$x));
#			printf("%10d", unpack("V", $x))
		}
#		printf("%s", unpack("h".$length*2, $data));
		print "\n";
	}
}


