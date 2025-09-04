<?php
if ( ! defined( 'ABSPATH' ) ) { exit; }

add_action( 'init', function() {
	register_post_type( 'asset', [
		'label'         => 'Assets',
		'public'        => true,
		'show_in_rest'  => true,
		'supports'      => [ 'title', 'editor' ],
		'menu_icon'     => 'dashicons-format-image',
		'template'      => [ [ 'wp3d/asset-editor' ] ],
		'template_lock' => 'insert',
	] );
	error_log( 'Registered CPT: asset' );
}, 20 );