<?php
// This file is generated. Do not modify it manually.
return array(
	'build' => array(
		'apiVersion' => 3,
		'name' => 'wp-3d-asset-editor/block',
		'title' => '3D Shared Scene',
		'category' => 'widgets',
		'icon' => 'dashicons-cover-image',
		'description' => 'Editable 3D scene for previewing game assets on the map.',
		'keywords' => array(
			'3d',
			'babylon',
			'leaflet',
			'model'
		),
		'version' => '1.0.0',
		'supports' => array(
			'align' => array(
				'wide',
				'full'
			),
			'html' => false
		),
		'attributes' => array(
			'blockAssetId' => array(
				'type' => array(
					'number',
					'null'
				),
				'default' => null
			),
			'assetUrl' => array(
				'type' => 'string',
				'default' => ''
			),
			'posX' => array(
				'type' => 'number',
				'default' => 0
			),
			'posY' => array(
				'type' => 'number',
				'default' => 0
			),
			'posZ' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotX' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotY' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotZ' => array(
				'type' => 'number',
				'default' => 0
			)
		),
		'editorScript' => 'file:./index.js',
		'editorStyle' => 'file:./style-index.css',
		'style' => 'file:./style-index.css'
	)
);
